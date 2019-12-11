package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/btwiuse/pretty"
	"github.com/remeh/sizedwaitgroup"
)

// stdin -> ScanFrom -> pd.in -> Run -> pd.out -> SendTo -> stdout

func main() {
	config := parseFlags()
	pd := New(config.Jobs, config.Template)
	go pd.ScanFrom(os.Stdin)
	go pd.SendTo(os.Stdout)
	pd.Run()
}

// Factory manufactores Job
func (j *Factory) Job(id string) *Job {
	return &Job{
		id:  id,
		url: fmt.Sprintf(j.template, id),
	}
}

type Factory struct {
	template string
	// timeout time.Duration
}

// Job downloads url
func (j *Job) Do() *Result {
	resp, err := http.Get(j.url)
	if err != nil {
		log.Println(err)
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
	id  string
	url string
}

// Config holds command line flags
func parseFlags() *Config {
	config := &Config{}
	flag.StringVar(&config.Template, "t", "", "url template, like https://example.com/%s.json")
	flag.IntVar(&config.Jobs, "j", 3, "parallel jobs")
	flag.Parse()
	return config
}

type Config struct {
	Jobs     int
	Template string
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
func New(j int, t string) *ParallelDownloader {
	return &ParallelDownloader{
		in:             make(chan string),
		out:            make(chan *Result),
		Factory:        &Factory{t},
		SizedWaitGroup: sizedwaitgroup.New(j),
	}
}

type ParallelDownloader struct {
	in  chan string
	out chan *Result
	*Factory
	sizedwaitgroup.SizedWaitGroup
}

func (p *ParallelDownloader) Run() {
	for { // id := range p.in { // avoid range here!!
		id := <-p.in
		// https://dave.cheney.net/2014/03/19/channel-axioms
		// A receive from a closed channel returns the zero value immediately
		// which implies io.EOF from Scan
		if id == "" {
			break
		}
		p.Add()
		go func() {
			defer p.Done()
			result := p.Job(id).Do()
			if result == nil {
				result = p.Job(id).Do()
				if result == nil {
					result = p.Job(id).Do()
					if result == nil {
						result = p.Job(id).Do()
						if result == nil {
							log.Println("retry failed:", id)
							return
						}
					}
				}
			}
			p.out <- result
		}()
	}
	p.Wait() // prevent pending goroutines sending to a closed channel
	// all results are written to p.out, but are they all written to logger?
	p.Add()
	close(p.out)
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
	}
	p.Done()
}
