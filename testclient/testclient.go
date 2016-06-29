package main

import "flag"
import "github.com/dpup/gohubbub"
import "log"

func main() {
	hubp := flag.String("hub", "http://localhost:8080/hub", "Hub to connect to")
	topicp := flag.String("topic", "http://localhost:8080/testrepo/events/push", "Topic to subscribe to")
	self := flag.String("self", "localhost:8000", "Address to call us back on")
	flag.Parse()

	client := gohubbub.NewClient(*self, "Testing")
	client.Subscribe(*hubp, *topicp, func(contentType string, body []byte) {
		log.Printf("Received callback %s\n%s\n", contentType, string(body))
	})
	client.StartAndServe("0.0.0.0", 54321)
}
