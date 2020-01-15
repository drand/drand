package net

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/drand/drand/protobuf/drand"
)

var _ PublicClient = (*restClient)(nil)

type restClient struct {
	marshaller runtime.Marshaler
	manager    *CertManager
}

// NewRestClient returns a client capable of calling external public method on
// drand nodes using the RESP API
func NewRestClient() PublicClient {
	return &restClient{
		marshaller: defaultJSONMarshaller,
		manager:    NewCertManager(),
	}
}

// NewRestClientFromCertManager returns a Rest Client with the given cert
// manager
func NewRestClientFromCertManager(c *CertManager) PublicClient {
	client := NewRestClient().(*restClient)
	client.manager = c
	return client
}

func (r *restClient) PublicRand(p Peer, in *drand.PublicRandRequest) (*drand.PublicRandResponse, error) {
	base := restAddr(p)
	var req *http.Request
	var err error
	basePath := base + "/api/public"
	if in.GetRound() == 0 {
		// then simple GET method
		req, err = http.NewRequest("GET", basePath, nil)
	} else {
		buff, err := r.marshaller.Marshal(in)
		if err != nil {
			return nil, err
		}
		url := fmt.Sprintf("%s/%d", basePath, in.GetRound())
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

func (r *restClient) PrivateRand(p Peer, in *drand.PrivateRandRequest) (*drand.PrivateRandResponse, error) {
	base := restAddr(p)
	buff, err := r.marshaller.Marshal(in)
	if err != nil {
		return nil, err
	}
	url := base + "/api/private"
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

func (r *restClient) DistKey(p Peer, in *drand.DistKeyRequest) (*drand.DistKeyResponse, error) {
	base := restAddr(p)
	buff, err := r.marshaller.Marshal(in)
	if err != nil {
		return nil, err
	}
	url := base + "/api/info/distkey"
	req, err := http.NewRequest("GET", url, bytes.NewBuffer(buff))
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(p, req)
	if err != nil {
		return nil, err
	}
	drandResponse := new(drand.DistKeyResponse)
	return drandResponse, r.marshaller.Unmarshal(respBody, drandResponse)
}

func (r *restClient) Group(p Peer, in *drand.GroupRequest) (*drand.GroupResponse, error) {
	base := restAddr(p)
	buff, err := r.marshaller.Marshal(in)
	if err != nil {
		return nil, err
	}
	url := base + "/api/info/group"
	req, err := http.NewRequest("GET", url, bytes.NewBuffer(buff))
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(p, req)
	if err != nil {
		return nil, err
	}
	drandResponse := new(drand.GroupResponse)
	return drandResponse, r.marshaller.Unmarshal(respBody, drandResponse)
}

func (r *restClient) Home(p Peer, in *drand.HomeRequest) (*drand.HomeResponse, error) {
	base := restAddr(p)
	buff, err := r.marshaller.Marshal(in)
	if err != nil {
		return nil, err
	}
	url := base + "/api"
	req, err := http.NewRequest("GET", url, bytes.NewBuffer(buff))
	if err != nil {
		return nil, err
	}
	respBody, err := r.doRequest(p, req)
	if err != nil {
		return nil, err
	}
	drandResponse := new(drand.HomeResponse)
	return drandResponse, r.marshaller.Unmarshal(respBody, drandResponse)

}

func (r *restClient) doRequest(remote Peer, req *http.Request) ([]byte, error) {
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}

	pool := r.manager.Pool()
	if remote.IsTLS() {
		h, _, err := net.SplitHostPort(remote.Address())
		if err != nil {
			return nil, err
		}
		conf := &tls.Config{
			RootCAs:    pool,
			ServerName: h,
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
