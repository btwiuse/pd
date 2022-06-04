package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/btwiuse/pd/swg"
	"github.com/btwiuse/pretty"
)

// data flow:
// os.Stdin -> ScanFrom -> pd.in -> Run(parallel download) -> pd.out -> SendTo -> os.Stdout

func main() {
	config := parseFlags()

	if config.PrintPid {
		log.Println("pid:", os.Getpid())
	}

	pd := New(config.Jobs, config.Template, config.Filter)

	if config.EnableReport {
		go pd.ReportStart(config.ReportInterval)
		defer pd.ReportStop()
	}

	pd.Run(os.Stdout, os.Stdin)
}

// Factory creates Job
func (j *Factory) Job(id string) *Job {
	return &Job{
		id:  id,
		url: fmt.Sprintf(j.template, id),
	}
}

type Factory struct {
	template string
}

// Job downloads url
func (j *Job) Do() *Result {
	j.start = time.Now()
	defer func() {
		j.end = time.Now()
	}()

	// default http client doesn't have timeout, here we add it
	// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 3 * time.Second,
		}).Dial,
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: 3 * time.Second,
	}
	var netClient = &http.Client{
		Timeout:   time.Second * 6,
		Transport: netTransport,
	}

	resp, err := netClient.Get(j.url)
	if err != nil {
		// log.Println(err)
		return nil
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return nil
	}
	return &Result{
		Id:    j.id,
		Value: string(body),
	}
}

type Job struct {
	start time.Time
	end   time.Time
	id    string
	url   string
}

// Config holds command line flags
func parseFlags() *Config {
	config := &Config{}
	flag.StringVar(&config.Template, "t", "https://hacker-news.firebaseio.com/v0/item/%s.json", "url template, like https://example.com/%s.json")
	flag.IntVar(&config.Jobs, "j", 3, "number of max parallel jobs")
	flag.BoolVar(&config.PrintPid, "p", false, "print pid")
	flag.BoolVar(&config.EnableReport, "r", false, "turn report on")
	flag.IntVar(&config.ReportInterval, "i", 1000, "report interval (in millisecond)")
	flag.StringVar(&config.Filter, "f", "Too Many Requests (HAP429).\n", "filter output")
	flag.Parse()
	return config
}

type Config struct {
	Jobs           int
	Template       string
	PrintPid       bool
	EnableReport   bool
	ReportInterval int
	Filter         string
}

// Result is Job result
type Result struct {
	Id    string `json:id`
	Value string `json:value`
}

func (r *Result) String() string {
	return pretty.JsonString(r)
	buf := new(bytes.Buffer)
	json.Compact(buf, []byte(pretty.JSONString(r)))
	return buf.String()
}

// ParallelDownloader wraps Qlock and Factory
func New(j int, t string, f string) *ParallelDownloader {
	lctx, lcancel := context.WithCancel(context.Background())
	return &ParallelDownloader{
		filter:    f,
		in:        make(chan string),
		out:       make(chan *Result),
		Factory:   &Factory{t},
		WaitGroup: swg.New(j),
		lctx:      lctx,
		lcancel:   lcancel,
	}
}

type ParallelDownloader struct {
	filter string
	in     chan string
	out    chan *Result
	*Factory
	*swg.WaitGroup
	counter        int
	prevcount      int
	retrycount     int
	prevretrycount int
	lctx           context.Context
	lcancel        context.CancelFunc
}

func (p *ParallelDownloader) Run(w io.Writer, r io.Reader) {
	go p.ScanFrom(r)
	go p.SendTo(w)
	for {
		// https://dave.cheney.net/2014/03/19/channel-axioms
		// A receive from a closed channel returns the zero value immediately
		// thus we know io.EOF is reached from ScanFrom and break the loop accordingly
		id := <-p.in
		if id == "" {
			break
		}

		/*
			// following parameter tuning slows down new job creation speed when job timeout/retry emerges,
			// so we have lower error rate. it applies specifically to the following use case
			//
			// $ cat ../containers | go run ./pd.go -r -j 50 -i 1000 -t https://hub.docker.com/v2/repositories/%s/dockerfile/ >dockerfiles
			//
			// not general enough to be useful for everyone but it's worth noting here for future reference
			// maybe I will find a better way to do such "Too Many Requests" error control
			switch k := float64(p.counter-p.prevcount)/float64(1+p.Len()); {
			case 0.5 < k && k <= 0.6:
				time.Sleep(250*time.Millisecond)
			case 0.6 < k && k <= 0.7:
				time.Sleep(360*time.Millisecond)
			case 0.7 < k && k <= 1:
				time.Sleep(500*time.Millisecond)
			case k >= 1:
				time.Sleep(600*time.Millisecond)
			}
			// if p.Len() > 20 { }
			time.Sleep(time.Duration(p.retrycount-p.prevretrycount) * 200 * time.Millisecond)
		*/

		p.Add()
		go func() {
			defer p.Done() // will always run under timeout/retry or normal return
			job := p.Job(id)
			var result *Result
			result = job.Do()
			// timeout or bad result
			if (result == nil) || (result.Value == p.filter) {
				return
				// retry after one second
				go func() {
					time.Sleep(1000 * time.Millisecond)
					p.in <- id
					// TODO: make ScanFrom wait id from this goroutine (if any) before it closes p.in
					// because sending to closed channel will make the program panic
					// if timeout/error rate is low enough, we are not likely to hit the bug
					// that's why we need error control
				}()
				p.retrycount++
				return
			}
			p.out <- result
			// p.stat <- job
		}()
	}
	// prevent pending job goroutines from sending result to a closed channel
	p.Wait()
	p.Add()
	close(p.out)
	// make sure all results are written to logger
	p.Wait()
}

func (p *ParallelDownloader) ScanFrom(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if len(id) == 0 {
			continue
		}
		p.in <- id
	}
	close(p.in)
}

func (p *ParallelDownloader) SendTo(w io.Writer) {
	logger := log.New(w, "", 0)
	for {
		result := <-p.out
		if result == nil {
			break
		}
		logger.Print(result)
		p.counter++
	}
	p.Done()
}

func (p *ParallelDownloader) ReportHead() {
	log.Printf("%8s %8s %8s %-8s +%-8s %-8s +%-8s\n", "cap", "len", "limit", "done", "diff", "rdone", "rdiff") // todo: cpu, mem, traffic, job duration histogram, moving average, average, eta, etc.
}

func (p *ParallelDownloader) ReportOnce() {
	current := p.counter
	diff := current - p.prevcount
	p.prevcount = current

	rcurrent := p.retrycount
	rdiff := rcurrent - p.prevretrycount
	p.prevretrycount = rcurrent
	log.Printf("%8d %8d %8d %-8d +%-8d %-8d +%-8d\n", p.Cap(), p.Len(), p.Limit, current, diff, rcurrent, rdiff)
}

func (p *ParallelDownloader) ReportStart(d int) {
	p.ReportHead()
	for {
		p.ReportOnce()
		select {
		case <-time.After(time.Duration(d) * time.Millisecond):
		case <-p.lctx.Done():
			return
		}
	}
}

func (p *ParallelDownloader) ReportStop() {
	p.lcancel()
	p.ReportOnce()
}
