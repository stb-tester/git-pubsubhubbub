package pushhub

import (
	"github.com/stretchr/testify/assert"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"
)

type TestHub struct {
	Address  string
	Hub      *Hub
	listener *net.IPConn
	t        *testing.T
}

func testSetup(t *testing.T, callback func(w http.ResponseWriter, r *http.Request)) *TestHub {
	hub := NewHub(
		*address,
		func(topic_ string) bool { return topic_ == topic },
		NullStore{})

	mux := http.NewServeMux()
	mux.HandleFunc("/hub", hub.HandleRequest)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Error("Listening on local hook callback port failed: %s\n", err)
	}

	go func() {
		err = http.Serve(listener, mux)
		if err != nil {
			t.Error("Failed to start HTTP server")
		}
	}()
	return TestHub{"http://" + listener.Addr().String() + "/hub",
		hub, listener, t}
}

type verificationRequest struct {
	callback_url string
	challenge    string
}

func checkVerificationRequest(t *testing.T, r *http.Request,
	subscribe bool,
	expectedTopic string, expectedURL string) verificationRequest {
	/* 5.3. Hub Verifies Intent of the Subscriber
	...
	The hub verifies a subscription request by sending an HTTP [RFC2616] GET
	request to the subscriber's callback URL as given in the subscription
	request. */
	assert.Equal(t, r.Method, "GET", "HTTP Method must be GET")

	/* ...This request has the following query string arguments appended
	(format described in Section 17.13.4 of [W3C.REC-html401-19991224]): */
	q := r.URL.Query()

	if expectedURL != "" {
		sr := r.URL
		sq := q.Query()
		sq.Del("hub.mode")
		sq.Del("hub.topic")
		sq.Del("hub.challenge")
		sq.Del("hub.lease_seconds")
		sr.RawQuery = sq.Encode()
		assert.Equal(t, sr.String(), expectedURL)
	}

	/* hub.mode
	REQUIRED. The literal string "subscribe" or "unsubscribe", which matches
	the original request to the hub from the subscriber. */
	if subscribe {
		assert.Equal(t, q.Get("hub.mode"), "subscribe", "")
	} else {
		assert.Equal(t, q.Get("hub.mode"), "unsubscribe", "")
	}

	/* hub.topic
	REQUIRED. The topic URL given in the corresponding subscription request. */
	assert.Nil(t, url.Parse(q.Get("hub.topic")[1], "hub.topic is not a valid url"))
	if expectedTopic != "" {
		assert.Equal(t, q.Get("hub.topic"), expectedTopic, "hub.topic incorrect")
	}

	/* hub.challenge
	REQUIRED. A hub-generated, random string that MUST be echoed by the
	subscriber to verify the subscription. */
	assert.NotEmpty(t, q.Get("hub.challenge"), "hub.challenge is not specified")

	/* hub.lease_seconds
	REQUIRED/OPTIONAL. The hub-determined number of seconds that the
	subscription will stay active before expiring, measured from the time the
	verification request was made from the hub to the subscriber. Hubs MUST
	supply this parameter for subscription requests. This parameter MAY be
	present for unsubscribe requests and MUST be ignored by subscribers during
	unsubscription.*/
	if subscribe {
		f, err := strconv.ParseFloat(q.Get("hub.lease_seconds"))
		assert.Nil(t, err, "hub.lease_seconds is not a number")
		assert.True(t, f >= 1, "hub.lease_seconds is too small")
	}

	return q.Get("hub.challenge")
}

type TestClient struct {
	listener *net.IPConn
	ch       chan HttpRequestHandler
	handled  chan int
}

type HttpRequestHandler struct {
	w    http.ResponseWriter
	r    *http.Request
	done chan int
}

func (tc *TestClient) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	done = make(chan int)
	tc.ch <- HttpRequestHandler{w, r, done}
	<-done
}

func SetupTestClient() {
	listener, err := net.Listen("tcp", "127.0.0.1:17145")
	if err != nil {
		t.Error("Listening on local hook callback port failed: %s\n", err)
	}

	tc = TestClient{listener, make(chan HttpRequestHandler)}

	go func() {
		err = http.Serve(listener, tc)
		if err != nil {
			t.Error("Failed to start HTTP server")
		}
	}()
	return tc
}

func TestVerification(t *testing.T) {
	ctx := testSetup(t)
	client := testClientSetup()

	/* 5.3.1. Verification Details
	...
	The hub MUST consider other server response codes (3xx, 4xx, 5xx) to mean
	that the verification request has failed. */
	for _, code := range []int{300, 400, 404, 500} {
		http.PostForm(ctx.Address, http.Values{
			"hub.mode":     "subscribe",
			"hub.callback": "http://127.0.0.1:17145/callback",
			"hub.topic":    "http://example.com/topic"})
		req := <-client.ch
		vr = checkVerificationRequest(t, req.r, true, "http://example.com/topic",
			"http://127.0.0.1:17145/callback")
		req.w.WriteHeader(300)
		req.w.Write([]byte(vr.challenge))
		req.w.done <- 1

	}

	/* If the subscriber returns an HTTP [RFC2616] success (2xx) but the content
	body does not match the hub.challenge parameter, the hub MUST also consider
	verification to have failed. */

}

func Test(t *testing.T) {
	ctx := testSetup(t)

}
