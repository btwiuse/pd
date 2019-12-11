package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	config := parseFlags()
	pd := New(config.Jobs, config.Template)
	go pd.Scan(os.Stdin)
	pd.Run()
}

// Push blocks when q is full
func (q Qlock) Push() {
	q <- struct{}{}
}

func (q Qlock) Pop() {
	<-q
}

type Qlock chan struct{}

// Factory manufactores Job
func (j *Factory) Job(id string) *Job {
	return &Job{
		id: id,
		url: fmt.Sprintf(j.template, id),
	}
}

type Factory struct {
	template string
	// timeout time.Duration
}

// Job downloads url
func (j *Job) Run() {
	resp, err := http.Get(j.url)
	if err != nil {
		log.Println(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	fmt.Println(j.id, string(body))
}

type Job struct {
	id string
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

// ParallelDownloader wraps Qlock and Factory
func New(j int, t string) *ParallelDownloader {
	return &ParallelDownloader{
		in:      make(chan string),
		out:     make(chan interface{}),
		Qlock:   make(chan struct{}, j),
		Factory: &Factory{t},
	}
}

type ParallelDownloader struct {
	in  chan string
	out chan interface{}
	Qlock
	*Factory
}

func (p *ParallelDownloader) Run() {
	for {
		select {
		case id := <-p.in:
			// https://dave.cheney.net/2014/03/19/channel-axioms
			// A receive from a closed channel returns the zero value immediately
			// which implies io.EOF from Scan
			if id == "" {
				return
			}
			p.Push()
			go func() {
				defer p.Pop()
				p.Job(id).Run()
			}()
			/*
				case <-p.ctx.Done():
					log.Println("context done")
					return
			*/
		}
	}
}

func (p *ParallelDownloader) Scan(r io.Reader) {
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
