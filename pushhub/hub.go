package pushhub

import (
	"bytes"
	"errors"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const LEASE_DURATION = 3 * time.Hour

type Hub struct {
	address string
	topicValidator func(topic string) bool
	store Store
	subscriptions map[string]map[string]Subscription
	mutex sync.Mutex
}

func NewHub(address string, topicValidator func(topic string) bool, store Store) Hub {
	return Hub{address, topicValidator, store,
	           map[string]map[string]Subscription{}, sync.Mutex{}}
}

type Subscription struct {

	/* These two are the "primary key" of a subscription */
	topic string
	callback url.URL

	/* Specified by them for secure callbacks: */
	secret string

	/* Specified by us to disconnect old clients: */
	lease_expires time.Time
};

func (sl Hub) Notify(topic string, mimetype string, payload []byte) error {
	now := time.Now()
	for_removal := []Subscription{}
	sl.mutex.Lock()
	defer sl.mutex.Unlock()
	for _, sub := range sl.subscriptions[topic] {
		if sub.lease_expires.Before(now) {
			for_removal = append(for_removal, sub)
			continue
		}
		go func() {
			sub_hmac := hmac.New(sha1.New, []byte(sub.secret))
			sub_hmac.Write(payload)
			x_hub_signature := "sha1=" + hex.EncodeToString(sub_hmac.Sum(nil))

			req, err := http.NewRequest("POST", sub.callback.String(),
				                        bytes.NewReader(payload))
			if err != nil {
				log.Printf("Failed to create POST request for %s", sub.callback.String())
				return
			}
			req.Header.Add("X-Hub-Signature", x_hub_signature)
			req.Header.Add("Content-Type", mimetype)
			req.Header.Add("Content-Length", fmt.Sprintf("%i", len(payload)))

			client := http.Client{}
			sleep_secs := time.Second
			for time.Now().Before(sub.lease_expires) {
				resp, err := client.Do(req)
				errmsg := ""

				if err == nil {
					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						return
					} else { 
						errmsg = fmt.Sprintf("status code %i", resp.StatusCode)
					}
				} else {
					errmsg = fmt.Sprintf("error %s", err)
				}

				log.Printf("Notifying %s failed with %s.  Retrying after %is", sub.callback.String(), errmsg, sleep_secs)

				/* Exponential backoff: */
				time.Sleep(sleep_secs)
				sleep_secs *= 2
			}
		} ();
	}

	if len(for_removal) > 0 {
		sl.store.Unsubscribe(for_removal)
		for _, v := range for_removal {
			delete(sl.subscriptions[topic], v.callback.String())
		}
	}
	return nil;
}

func verify(mode string, sub Subscription) error {
	challenge_bytes := make([]byte, 32)
	if _, err := rand.Read(challenge_bytes); err != nil {
		return err
	}
	/* I want our challenge to be a string of ascii to reduce the chances that
	   we trip clients up with escape sequences or URL encoding, etc. */
	challenge := hex.EncodeToString(challenge_bytes);

	request_url := sub.callback
	q := request_url.Query()
	q.Set("hub.mode", mode);
	q.Set("hub.topic", sub.topic);
	q.Set("hub.challenge", challenge);
	q.Set("hub.lease_seconds",
	      fmt.Sprintf("%d", int(sub.lease_expires.Sub(time.Now()).Seconds())));
	request_url.RawQuery = q.Encode()

	res, err := http.Get(request_url.String())
	if err != nil {
		log.Printf("INFO: Verification failed: GETting callback URL %s failed: %s", sub.callback.String(), err)
		return err
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)

	if !bytes.Equal(body, []byte(challenge)) {
		log.Printf("INFO: Verification failed: Callback URL %s did not respond with challenge.  Received %s instead\n", sub.callback, body)
		return errors.New("Verification Failed")
	}

	log.Printf("INFO: Verification of %s %s, %s succeeded\n", mode, sub.topic, sub.callback.String());
	return nil;
}

func (hub Hub) HandleRequest(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: Received request %s", *r)

	if r.Method != "POST" {
		http.Error(w, fmt.Sprintf("Invalid method '%s'.  You must use method 'POST'", r.Method), http.StatusMethodNotAllowed)
		return
	}
	mode := r.FormValue("hub.mode")
	topic := r.FormValue("hub.topic")
	callback := r.FormValue("hub.callback")
	secret := r.FormValue("hub.secret")
	/* hub.lease_seconds is optional, we just stick to the server default
	 * lease_seconds := r.FormValue("hub.lease_seconds") */

	if !hub.topicValidator(topic) {
		log.Print("INFO: ", mode, " Failed: Unknown topic ", topic)
		http.Error(w, fmt.Sprintf("Unknown topic '%s'", topic), http.StatusBadRequest)
		return
	}

	parsed_url, err := url.Parse(callback)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid callback specified - Not a valid URL: %s.  We got callback '%s'", err, callback), http.StatusBadRequest)
		return
	}
	if parsed_url.Scheme != "http" && parsed_url.Scheme != "https" {
		http.Error(w, fmt.Sprintf("Invalid callback specified.  Scheme must be http or https.  We got '%s' from callback '%s'", parsed_url.Scheme, callback), http.StatusBadRequest)
		return
	}

	if mode != "subscribe" && mode != "unsubscribe" {
		http.Error(w, fmt.Sprintf("Invalid mode: %s", mode), http.StatusBadRequest)
		return
	}

	sub := Subscription{topic, *parsed_url, secret, time.Now().Add(LEASE_DURATION)};

	w.WriteHeader(http.StatusAccepted);
	w.Write([]byte(fmt.Sprintf("pubsubhubbub %s accepted, verifying", mode)))

	if err := verify(mode, sub); err != nil {
		log.Printf("INFO: Verifiying %s request for (%s, %s) failed: %s", mode, topic, callback, err)
		return;
	}

	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	switch mode {
	case "subscribe":
		log.Print("INFO: Subscription successful: ", sub.callback)
		if hub.subscriptions[sub.topic] == nil {
			hub.subscriptions[sub.topic] = map[string]Subscription{}
		}
		hub.subscriptions[sub.topic][sub.callback.String()] = sub
		hub.store.Subscribe([]Subscription{sub});
	case "unsubscribe":
		log.Print("INFO: Unsubscription successful: ", sub.callback)
		delete(hub.subscriptions[sub.topic], sub.callback.String())
		if len(hub.subscriptions[sub.topic]) == 0 {
			delete(hub.subscriptions, sub.topic)
		}
		hub.store.Unsubscribe([]Subscription{sub});
	}
}
