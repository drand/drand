package net

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/dedis/drand/protobuf/drand"
)

type restClient struct {
	marshaller runtime.Marshaler
	manager    *CertManager
}

func NewRestClient() ExternalClient {
	return &restClient{
		marshaller: defaultJSONMarshaller,
		manager:    NewCertManager(),
	}
}

func NewRestClientFromCertManager(c *CertManager) ExternalClient {
	client := NewRestClient().(*restClient)
	client.manager = c
	return client
}

func (r *restClient) Public(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	base := restAddr(p)
	var req *http.Request
	var err error
	if in.GetRound() == 0 {
		// then simple GET method
		req, err = http.NewRequest("GET", base+"/public", nil)
	} else {
		buff, err := r.marshaller.Marshal(in)
		if err != nil {
			return nil, err
		}
		url := fmt.Sprintf("%s/public/%d", base, in.GetRound())
		req, err = http.NewRequest("GET", url, bytes.NewBuffer(buff))
	}
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(p, req)
	if err != nil {
		return nil, err
	}
	drandResponse := new(drand.PublicRandResponse)
	return drandResponse, r.marshaller.Unmarshal(respBody, drandResponse)
}

func (r *restClient) Private(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	base := restAddr(p)
	buff, err := r.marshaller.Marshal(in)
	if err != nil {
		return nil, err
	}
	url := base + "/private"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(buff))
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(p, req)
	if err != nil {
		return nil, err
	}
	drandResponse := new(drand.PrivateRandResponse)
	return drandResponse, r.marshaller.Unmarshal(respBody, drandResponse)

}

func (r *restClient) doRequest(remote Peer, req *http.Request) ([]byte, error) {
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}

	pool := r.manager.Pool()
	if remote.IsTLS() {
		conf := &tls.Config{
			RootCAs:    pool,
			ServerName: remote.Address(),
		}
		client.Transport = &http.Transport{TLSClientConfig: conf}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(resp.Body)
}

func restAddr(p Peer) string {
	if p.IsTLS() {
		return "https://" + p.Address()
	}
	return "http://" + p.Address()
}
