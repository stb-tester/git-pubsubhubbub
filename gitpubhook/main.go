package main

import (
	"path/filepath"
	"flag"
	"fmt"
	"net/http"
	"git-pubsubhubbub/pushhub"
	"os"
	"os/exec"
)

func serve() int {
	prefixp := flag.String("prefix", "", "HTTP endpoint prefix");
	address := flag.String("address", ":8080", "Address to listen on");

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

	/* Generate nonce that will be used to confirm that HTTP POSTs in to this
	 * process are from the git hooks.  This is saved with great care with
	 * respect to permissions. */
	nonce_bytes := make([]byte, 32)
	if _, err := rand.Reader(nonce); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to generate nonce: %s", err)
		return 1
	}
	nonce := hex.EncodeToString(nonce_bytes);

	/* We want users who can write to the repo to be able to call the hook, but
	 * no-one else, so copy the write mode on the git directory to the read mode
	 * of the nonce file */
	if git_stat, err := os.Stat(git_dir); err != nil {
		fmt.Fprintln(os.Stderr, "statting git dir failed: %s", err)
		return 1
	}

	nonce_mode := 0200 | ((git_stat.Mode() & 0222) << 1)
	err := writeFileExcl("git-pubsubhubbub-nonce.txt", nonce, nonce_mode,
	                     git_stat.Uid, git_stat.Gid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to write pubsubhubbub nonce: %s", err)
		return 1
	}

	fmt.Printf("Serving pubsubhubbub on http://%s%s/push\n", *address, prefix)

	hub := pushhub.Hub{
		*address,
		func (topic string) bool {return topic == prefix + "/push"},
		pushhub.NullStore{},
		map[string]map[string]pushhub.Subscription{},
	}

	http.HandleFunc(prefix + "/push", hub.HandleRequest)

	onHook := func (w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, fmt.Sprintf("Invalid method '%s'.  You must use method 'POST'", r.Method), http.StatusMethodNotAllowed)
			return
		}
		
	}

	http.HandleFunc(prefix + "/push/hooks/pre-receive", handleHook(nonce))

	if err := http.ListenAndServe(hub.address, nil); err != nil {
		fmt.Fprintln(os.Stderr, "Serving failed: ", err);
		os.Exit(1);
	}
}


func writeFileExcl(name string, data []byte, perm os.FileMode, uid int, gid int) error {
	if err := os.RemoveAll(name); err != nil {
		return err
	}

	f, err := os.OpenFile(name, os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0200)
	if err != nil {
		return err
	}
	defer f.Close()

	if statinfo, err := f.Stat(); err != nil {
		return err
	}

	if statinfo.Uid != uid || statinfo.Gid != gid {
		if err := f.chown(uid, gid); err != nil {
			fmt.Fprintln(os.Stderr, "Cannot create nonce file: We must be running with same uid and gid as the git dir, be running as root or have the CAP_CHOWN capability: %s", err);
			os.Remove(name)
			return err
		}
	}

	if err := f.chmod(perm); err != nil {
		os.Remove(name)
		return err
	}

	if _, err := f.Write(data); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

/* Called by git when a git-push is complete.  We need to be able to contact
 * our server to pass on the information that git sends us such that it can
 * distribute that data to all the subscribers.  See `man githooks` for more
 * details. */
func hook() int {
	if nonce, err := ioutil.ReadFile("git-pubsubhubbub-nonce.txt"); err != nil {
		fmt.Fprintln(os.Stderr, "hook failed to read pubsubhubbub nonce")
		return 1
	}

	req, err := http.NewRequest("POST", endpoint, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create POST request for %s", endpoint)
		return 1
	}
	req.Header.Add("X-Git-Pubsubhubbub-Nonce", nonce)
	req.Header.Add("Content-Type", "application/octet-stream")

	client := http.Client{}
	if resp, err := client.Do(req); err != nil {
		fmt.Fprintln(os.Stderr, "HTTP POST to pubsubhubbub hub failed")
		return 1
	}

	if resp.StatusCode != 200 {
		/* We're in an error state, don't care about further errors: */
		io.Copy(os.Stderr, resp.Body)
		return 1
	}

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		fmt.Fprintln(os.Stderr, "hook failed to read HTTP response")
		return 1
	}
	return 0
}

func handleHook(nonce string, topic string, hub *Hub) {
	/* Called in response to receiving a POST from hook() */
	return func (w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		r_nonce := r.Header.Get("X-Git-Pubsubhubbub-Nonce")
		if r_nonce != nonce {
			w.WriteHeader(403)
			w.Write("Incorrect Nonce")
			return
		}

		if payload, err := ioutil.ReadAll(r.Body); err != nil {
			w.WriteHeader(400)
			w.Write("Failed to read payload from body")
			return
		}

		if err := hub.Notify(topic, "application/json", payload); err != nil {
			w.WriteHeader(500)
			w.Write("Notifing subscribers failed")
			return
		}
		w.WriteHeader(200)
	}
}

func main() {
	if path.Base(os.Args[0]).HasPrefix("post-receive") {
		os.Exit(hook())
	} else {
		os.Exit(serve())
	}
}
