package net

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/dedis/drand/protobuf/drand"
)

type restClient struct {
	marshaller runtime.Marshaler
}

func NewRestClient() ExternalClient {
	return &restClient{defaultJSONMarshaller}
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
	respBody, err := r.doRequest(req)
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
	respBody, err := r.doRequest(req)
	if err != nil {
		return nil, err
	}
	drandResponse := new(drand.PrivateRandResponse)
	return drandResponse, r.marshaller.Unmarshal(respBody, drandResponse)

}

func (r *restClient) doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(resp.Body)
}

func restAddr(p Peer) string {
	if IsTLS(p.Address()) {
		return "https://" + p.Address()
	}
	return "http://" + p.Address()
}
