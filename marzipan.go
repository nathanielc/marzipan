package marzipan

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type ClientConfig struct {
	Host     string
	User     string
	Password string
	Logger   *log.Logger
}

type Client struct {
	mu        sync.RWMutex
	wg        sync.WaitGroup
	conn      *websocket.Conn
	logger    *log.Logger
	requests  chan request
	responses chan Response
	idx       int
	closing   chan struct{}
	subs      map[string][]chan Response
}

type request struct {
	MobileInternalIndex string
	Request             Request
	ResponseC           chan Response
}

func NewClient(c ClientConfig) (*Client, error) {
	u := url.URL{
		Scheme: "ws",
		Host:   c.Host,
		Path:   path.Join("/", c.User, c.Password),
	}

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(u.String(), http.Header{
		"Origin": []string{"local.host"},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s", c.Host)
	}
	logger := c.Logger
	if logger == nil {
		logger = log.New(ioutil.Discard, "", log.LstdFlags)
	}
	cli := &Client{
		conn:      conn,
		closing:   make(chan struct{}),
		requests:  make(chan request),
		responses: make(chan Response),
		subs:      make(map[string][]chan Response),
		logger:    logger,
	}
	cli.wg.Add(1)
	go func() {
		defer cli.wg.Done()
		cli.run()
	}()
	cli.wg.Add(1)
	go func() {
		defer cli.wg.Done()
		cli.readLoop()
	}()
	return cli, nil
}

func (c *Client) Close() {
	c.conn.Close()
	close(c.closing)
	c.wg.Wait()
}

func (c *Client) Do(r Request) (string, <-chan Response, error) {
	c.mu.Lock()
	c.idx++
	mii := strconv.Itoa(c.idx)
	c.mu.Unlock()

	r.SetMobileInternalIndex(mii)
	req := request{
		MobileInternalIndex: mii,
		Request:             r,
		ResponseC:           make(chan Response, 1),
	}
	select {
	case c.requests <- req:
	case <-c.closing:
		return "", nil, errors.New("client closed")
	}
	return req.MobileInternalIndex, req.ResponseC, nil
}

func (c *Client) Subscribe(ct string) <-chan Response {
	sub := make(chan Response, 10)
	c.mu.Lock()
	c.subs[ct] = append(c.subs[ct], sub)
	c.mu.Unlock()
	return sub
}

func (c *Client) readLoop() {
	log.Println("Start readLoop")
	for {
		r := make(genericResponse)
		if err := c.conn.ReadJSON(&r); err != nil {
			c.logger.Println("failed to read response JSON:", err)
			return
		}
		response, err := r.Response()
		if err != nil {
			c.logger.Println("failed to unmarshal response JSON:", err)
			continue
		}
		select {
		case c.responses <- response:
		case <-c.closing:
			return
		}
	}
}

func (c *Client) run() {
	activeRequests := make(map[string]chan Response)
	for {
		select {
		case <-c.closing:
			return
		case r := <-c.requests:
			activeRequests[r.MobileInternalIndex] = r.ResponseC
			b, _ := json.Marshal(r.Request)
			log.Println("write Request", string(b))
			if err := c.conn.WriteJSON(r.Request); err != nil {
				c.logger.Printf("failed to write request JSON. MII: %s Error: %v", r.MobileInternalIndex, err)
			}
		case r := <-c.responses:
			mii := r.MobileInternalIndex()
			if rc, ok := activeRequests[mii]; ok {
				rc <- r
				delete(activeRequests, mii)
			}
			ct := r.CommandType()
			c.mu.RLock()
			subs := c.subs[ct]
			c.mu.RUnlock()
			for _, sub := range subs {
				select {
				case sub <- r:
				default:
				}
			}
		}
	}
}

type genericResponse map[string]*json.RawMessage

func (r genericResponse) Meta() Meta {
	return Meta{
		ACT: r.string(Action),
		MII: r.string(MobileInternalIndex),
		CT:  r.string(CommandType),
	}
}

func (r genericResponse) string(k string) string {
	raw, ok := r[k]
	if !ok {
		return ""
	}
	var s string
	json.Unmarshal(*raw, &s)
	return s
}

func (r genericResponse) unmarshal(k string, v interface{}) error {
	raw, ok := r[k]
	if !ok {
		return fmt.Errorf("unknown key %q", k)
	}
	return json.Unmarshal(*raw, v)
}

func (r genericResponse) Response() (Response, error) {
	commandType := r.string(CommandType)
	switch commandType {
	case "DeviceList":
		dl := &DeviceList{
			Meta:    r.Meta(),
			Devices: make(map[string]Device),
		}
		err := r.unmarshal("Devices", &dl.Devices)
		return dl, err
	case "DynamicIndexUpdated":
		diu := &DynamicIndexUpdated{
			Meta:    r.Meta(),
			Devices: make(map[string]Device),
		}
		err := r.unmarshal("Devices", &diu.Devices)
		return diu, err
	default:
		return nil, fmt.Errorf("unsupported command type: %q", commandType)
	}
}
