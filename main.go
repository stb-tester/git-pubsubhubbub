package main

import (
	"path/filepath"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

func serve(address, prefix) {
}

func hook(endpoint, nonce) {
	req, err := http.NewRequest("POST", endpoint, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create POST request for %s", endpoint)
		return 1
	}
	req.Header.Add("X-Git-Pubsubhubbub-Nonce", nonce)
	req.Header.Add("Content-Type", "text/plain")

	client := http.Client{}
	resp, err := client.Do(req)
	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		fmt.Fprintln(os.Stderr, "hook failed to read HTTP response")
		return 1
	}
}

func main() {
	prefixp := flag.String("prefix", "", "HTTP endppoint prefix");
	address := flag.String("address", ":8080", "Address to listen on");

	nonce := flag.String("-nonce", "", "HTTP endppoint prefix");
	url := flag.String("-address", ":8080", "Address to listen on");

	flag.Parse();

	git_dir, err := exec.Command("git", "rev-parse", "--git-dir").Output();
	if err != nil {
		fmt.Fprintln(os.Stderr, "git failed: ", err);
		os.Exit(1);
	}

	toplevelb, err := exec.Command("git", "rev-parse", "--show-toplevel").Output();
	if err != nil {
		fmt.Fprintln(os.Stderr, "git failed: ", err);
		os.Exit(1);
	}
	toplevel := string(toplevelb[:len(toplevelb)-1]); /* strip trailing newline */

	if (toplevel == "") {
		/* This can happen if run from a bare repo */
		if toplevel, err = filepath.Abs(string(git_dir)); err != nil {
			fmt.Fprintln(os.Stderr, "filepath.Abs failed: ", err);
			os.Exit(1);
		}
	}

	repo_name := filepath.Base(toplevel);
	var prefix string;
	if *prefixp != "" {
		prefix = *prefixp;
	} else {
		prefix = "/" + repo_name + "/events"
	} 

	fmt.Printf("Serving pubsubhubbub on http://%s%s/push\n", *address, prefix)

	hub := Hub{
		*address,
		func (topic string) bool {return topic == prefix + "/push"},
		NullStore{},
		map[string]map[string]Subscription{},
	}

	http.HandleFunc(prefix + "/push", hub.HandleRequest)

	onHook := func (w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, fmt.Sprintf("Invalid method '%s'.  You must use method 'POST'", r.Method), http.StatusMethodNotAllowed)
			return
		}
		
	}

	http.HandleFunc(prefix + "/push/hooks/pre-receive", handleHookPost)

	if err := http.ListenAndServe(hub.address, nil); err != nil {
		fmt.Fprintln(os.Stderr, "Serving failed: ", err);
		os.Exit(1);
	}
}
