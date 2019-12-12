# pd - Parallel Downloader

I made this program mainly for following purposes
- scraping Hacker News json api (Example: https://hacker-news.firebaseio.com/v0/item/2.json)
- scraping Dockerfiles from Docker Hub api (Example: https://hub.docker.com/v2/repositories/navigaid/mathador/dockerfile/)

I'm pretty sure there are other solutions out there that are way better than mine, but hey, they are 'Not invented here' ;)
And if you find one, please let me know by opening a issue :)

## Design
`pd` is made to work with Unix pipeline, it reads input from stdin line by line, each line is assigned to a variable called `id`, and each `id` will be applied against a predefined template, usually of the form `https://hacker-news.firebaseio.com/v0/item/%s.json`, producing a valid url, for each url `pd` will spawn a goroutine to download the content, and then the response body `resp` is encoded into a single line json along with `id`, in the format `{"Id": id, "Value", resp}`

Internally `pd` makes heavy use of channel operations, articles on this can be found in the References section. The `ParallelDownloader` structs has two channels, it reads `id`s from the `in` channel and send results to the `out` channel. In the middle `pd` uses something called "sized wait group" or `swg` to do job control, defined under the `./swg` directory. Basically a `swg` is like normal `sync.WaitGroup` but `Add()` blocks when it reaches a certain limit.

`pd` doesn't save result to separate files, instead, each result along with its `id` is encoded to a json line, then written to stdout. You need to sort and parse the output on your own. `jq` is your friend.

## Install
```bash
$ go get -u github.com/btwiuse/pd
```

## Usage
```bash
$ go run pd.go -h
Usage of pd:
  -f string
        filter output (default "Too Many Requests (HAP429).\n")
  -i int
        report interval (in millisecond) (default 1000)
  -j int
        number of max parallel jobs (default 3)
  -p    print pid
  -r    turn report on
  -t string
        url template, like https://example.com/%s.json (default "https://hacker-news.firebaseio.com/v0/item/%s.json")
```

the number of maximum parallel downloading goroutines is limited to 3 by default, you can customize the value with `-j` parameter

WARNING: 
If `-j` is set too high you might be rate limited or get you IP blocked by the api server, or even take down the service. You might be conducting a DDoS attack unknowingly, and bear legal consequences, so remember to be gentle and kind when you use `pd`. 

Disclaimer: 
I'm not responsible for any forementioned consequences you made with `pd`

## Examples
Interactive use(for test purpose)
```
$ pd -t https://hacker-news.firebaseio.com/v0/item/%s.json
1
{"Id":"1","Value":"{\"by\":\"pg\",\"descendants\":15,\"id\":1,\"kids\":[15,234509,487171,454426,454424,454410,82729],\"score\":57,\"time\":1160418111,\"title\":\"Y Combinator\",\"type\":\"story\",\"url\":\"http://ycombinator.com\"}"}
2
{"Id":"2","Value":"{\"by\":\"phyllis\",\"descendants\":0,\"id\":2,\"kids\":[454411],\"score\":16,\"time\":1160418628,\"title\":\"A Student's Guide to Startups\",\"type\":\"story\",\"url\":\"http://www.paulgraham.com/mit.html\"}"}
```

Usually you should pipe `id`s separated by new line to `pd` redirect the stdout of pd to a file
```
seq 42 | go run pd.go -j 8 -t https://hacker-news.firebaseio.com/v0/item/%s.json > hndump
```

Then you can use `jq` to manipulate the json format output
```
$ cat hndump | jq -c .
{"Id":"7","Value":"{\"by\":\"phyllis\",\"descendants\":0,\"id\":7,\"kids\":[454416],\"score\":5,\"time\":1160420455,\"title\":\"Sevin Rosen Unfunds - why?\",\"type\":\"story\",\"url\":\"http://featured.gigaom.com/2006/10/09/sevin-rosen-unfunds-why/\"}"}

... ...

{"Id":"41","Value":"{\"by\":\"starklysnarky\",\"id\":41,\"kids\":[454461],\"parent\":37,\"text\":\"it's interesting how a simple set of features can make the product seem wholly different, new, and interesting. that, and the use of 'revolutionary' and 'incredible' everywhere. =)\",\"time\":1160520368,\"type\":\"comment\"}"}
{"Id":"42","Value":"{\"by\":\"sergei\",\"descendants\":0,\"id\":42,\"kids\":[28355,28717,454463],\"score\":5,\"time\":1160532601,\"title\":\"An alternative to VC: &#34;Selling In&#34;\",\"type\":\"story\",\"url\":\"http://www.venturebeat.com/contributors/2006/10/10/an-alternative-to-vc-selling-in/\"}"}
```

the output is unsorted because they are downloaded in parallel, `pd` only guarantees the result of each id is written to output before exit.

## References
- https://dave.cheney.net/2014/03/19/channel-axioms
- http://blog.golang.org/pipelines
