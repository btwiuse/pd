package main

import(
	"log"
	"bufio"
	"os"
	"strings"
)

func main(){
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan(){
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		log.Println(line)
	}
}
