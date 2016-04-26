package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"encoding/hex"
	"git-pubsubhubbub/pushhub"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

func serve() int {
	address := flag.String("listen", ":8080", "Address to listen on");
	hub_endpoint := flag.String("hub-endpoint", "/hub", "The endpoint to which subscription requests must be sent");
	topic_prefix := flag.String("topic-prefix", "http://localhost:8080/", "HTTP endpoint prefix under which your topics will appear")

	flag.Parse();

	git_dir, err := gitDir()
	if err != nil {
		log.printf("FATAL: git failed: %s\n");
		return 1
	}

	toplevelb, err := exec.Command("git", "rev-parse", "--show-toplevel").Output();
	if err != nil {
		log.Printf("FATAL: git failed: %s\n", err);
		return 1
	}
	var toplevel string;
	if (toplevel == "") {
		/* This can happen if run from a bare repo */
		if toplevel, err = filepath.Abs(string(git_dir)); err != nil {
			log.Printf("FATAL: filepath.Abs failed: %s\n", err);
			return 1
		}
	} else {
		toplevel = string(toplevelb[:len(toplevelb)-1]); /* strip trailing newline */
	}

	repo_name := filepath.Base(toplevel);
	topic := *topic_prefix + repo_name + "/events/push"

	/* Generate nonce that will be used to confirm that HTTP POSTs in to this
	 * process are from the git hooks.  This is saved with great care with
	 * respect to permissions. */
	nonce_bytes := make([]byte, 32)
	if _, err := rand.Read(nonce_bytes); err != nil {
		log.Printf("FATAL: Failed to generate nonce: %s", err)
		return 1
	}
	nonce := hex.EncodeToString(nonce_bytes);

	hub := pushhub.NewHub(
		*address,
		func (topic_ string) bool {return topic_ == topic},
		pushhub.NullStore{})

	hookCallbackAddress, err := listenForHookCallbacks(
	    nonce, topic, &hub)
	if err != nil {
		log.Fatal("Listening for hook callbacks on localhost failed", err)
		os.Exit(1)
	}
	log.Print("Listening for hook callbacks on ", hookCallbackAddress)

	/* We want users who can write to the repo to be able to call the hook, but
	 * no-one else, so copy the write mode on the git directory to the read mode
	 * of the nonce file */
	git_stat, err := os.Stat(string(git_dir))
	if err != nil {
		log.Printf("FATAL: statting git dir failed: %s\n", err)
		return 1
	}

	nonce_filename := string(git_dir) + "/git-pubsubhubbub.txt"
	nonce_mode := 0200 | ((git_stat.Mode() & 0222) << 1)
	err = writeFileExcl(nonce_filename,
	                    []byte("http://" + hookCallbackAddress + "/ " + nonce),
	                    nonce_mode,
	                    int(git_stat.Sys().(*syscall.Stat_t).Uid),
	                    int(git_stat.Sys().(*syscall.Stat_t).Gid))
	if err != nil {
		log.Printf("FATAL: Failed to write pubsubhubbub nonce: %s", err)
		return 1
	}
	defer func() {
		os.Remove(nonce_filename)
	}()

	/* Install ourselves as a hook */
	selfpath, err := filepath.Abs(os.Args[0])
	if err != nil {
		log.Printf("FATAL: Couldn't work out path to this executable: %s\n", err)
		return 1
	}

	hook := string(git_dir) + "/hooks/post-receive"
	if err := os.MkdirAll(filepath.Dir(hook), 0755); err != nil {
		log.Fatal("Couldn't make hooks directory: ", err)
		return 1
	}

	link, err := os.Readlink(hook)
	if err != nil && !os.IsNotExist(err) || err == nil && link != selfpath {
		log.Fatal("post-receive hook already exists in this repo, refusing to ",
		          "create one")
		return 1
	}
	if link != selfpath {
		if err := os.Symlink(selfpath, hook); err != nil {
			log.Fatal("Couldn't create hook ", hook, ": ", err)
			return 1
		}
	}
	defer func() {
		if l, _ := os.Readlink(hook); l == selfpath {
			os.Remove(l)
		}
	}()

	fmt.Printf("Serving pubsubhubbub on http://%s%s\n", *address, *hub_endpoint)
	fmt.Printf("Available topics:\n    %s\n", topic)

	http.HandleFunc(*hub_endpoint, hub.HandleRequest)

	if err := http.ListenAndServe(*address, nil); err != nil {
		log.Printf("FATAL: Serving failed: %s\n", err);
		return 1
	}
	return 0
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

	statinfo, err := f.Stat()
	if err != nil {
		return err
	}

	if (int(statinfo.Sys().(*syscall.Stat_t).Uid) != uid ||
	        int(statinfo.Sys().(*syscall.Stat_t).Gid) != gid) {
		if err := f.Chown(uid, gid); err != nil {
			log.Printf("FATAL: Cannot create nonce file: We must be running with same uid and gid as the git dir, be running as root or have the CAP_CHOWN capability: %s\n", err);
			os.Remove(name)
			return err
		}
	}

	if err := f.Chmod(perm); err != nil {
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
	git_dir, err := gitDir()
	if err != nil {
		log.Printf("FATAL: Couldn't find git dir: %s\n", err);
		return 1
	}

	data, err := ioutil.ReadFile(git_dir + "/git-pubsubhubbub.txt")
	if err != nil {
		fmt.Printf("FATAL: hook failed to read pubsubhubbub callback file: %s.  Pubsubhubbub notification may not have been sent.\n", err)
		return 1
	}
	a := strings.Split(string(data), " ")
	endpoint := a[0]
	nonce := a[1]

	req, err := http.NewRequest("POST", endpoint, os.Stdin)
	if err != nil {
		log.Printf("FATAL: Failed to create POST request for %s\n", endpoint)
		return 1
	}
	req.Header.Add("X-Git-Pubsubhubbub-Nonce", nonce)
	req.Header.Add("Content-Type", "application/octet-stream")

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("FATAL: HTTP POST to pubsubhubbub hub failed\n", err)
		return 1
	}

	if resp.StatusCode != 200 {
		/* We're in an error state, don't care about further errors: */
		log.Printf("FATAL: POSTing to pubsubhubbub hub failed.  Pubsubhubbub notification may not have been sent.  Server said:\n")
		io.Copy(os.Stderr, resp.Body)
		return 1
	}

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		log.Printf("WARN: hook failed to read HTTP response. Pubsubhubbub notification may not have been sent\n")
		return 1
	}
	return 0
}

func listenForHookCallbacks(nonce string, topic string, hub *pushhub.Hub) (string, error) {
	/* Called in response to receiving a POST from hook() */
	callback := func (w http.ResponseWriter, r *http.Request) {
		log.Printf("INFO: Received hook callback from %s\n", r.RemoteAddr)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		r_nonce := r.Header.Get("X-Git-Pubsubhubbub-Nonce")
		if r_nonce != nonce {
			w.WriteHeader(403)
			w.Write([]byte("Incorrect Nonce"))
			log.Printf("Warning: Incorrect nonce %s received", r_nonce)
			return
		}

		payload, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("Failed to read payload from body"))
			log.Print("Warning: Failed to read payload from body")
			return
		}

		if err := hub.Notify(topic, "application/json", payload); err != nil {
			w.WriteHeader(500)
			w.Write([]byte("Notifing subscribers failed"))
			log.Print("Warning: Notifing subscribers failed: ", err)
			return
		}
		w.WriteHeader(200)
		log.Print("Info: Hook callback: OK")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", callback)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("FATAL: Listening on local hook callback port failed: %s\n", err)
		return "", err
	}

	go func () {
		err = http.Serve(listener, mux)
		if err != nil {
			log.Fatal("FATAL: http.Serve on local hook callback failed: ", err)
		}
	} ()
	return listener.Addr().String(), nil
}

func gitDir() (string, error) {
	git_dir, err := exec.Command("git", "rev-parse", "--git-dir").Output();
	if err != nil {
		log.Printf("WARN: git failed: %s\n", err);
		return "", err
	}
	return string(git_dir[:len(git_dir)-1]), nil /* strip trailing newline */
}

func main() {
	if strings.HasPrefix(path.Base(os.Args[0]), "post-receive") {
		os.Exit(hook())
	} else {
		os.Exit(serve())
	}
}
