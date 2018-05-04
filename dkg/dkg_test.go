package dkg

import (
	"sync"
	"testing"
	"time"

	"github.com/nikkolasg/dsign/key"
	"github.com/nikkolasg/dsign/net"
	"github.com/nikkolasg/dsign/test"
)

var encoder = net.NewSingleProtoEncoder(&Packet{})

type network struct {
	gw  net.Gateway
	dkg *Handler
	cb  func()
}

func newDkgNetwork(gw net.Gateway, priv *key.Private, conf *Config, cb func()) *network {
	n := &network{
		gw: gw,
	}
	//fmt.Printf("Starting gateway %p\n", &gw)
	gw.Start(n.Process)
	n.dkg = NewHandler(priv, conf, n)
	go func() {
		select {
		case <-n.dkg.WaitShare():
			//fmt.Printf("waitshare DONE for gateway %p\n", &gw)
			cb()
		case e := <-n.dkg.WaitError():
			panic(e)
		}
	}()
	return n
}

func (n *network) Send(id *key.Identity, p *Packet) error {
	buff, err := encoder.Marshal(p)
	if err != nil {
		return err
	}
	return n.gw.Send(id, buff)
}

func (n *network) Process(from *key.Identity, msg []byte) {
	packet, err := encoder.Unmarshal(msg)
	if err != nil {
		return
	}
	dkgPacket := packet.(*Packet)
	n.dkg.Process(from, dkgPacket)
}

func networks(keys []*key.Private, gws []net.Gateway, threshold int, cb func(), timeout time.Duration) []*network {
	list := test.ListFromPrivates(keys)
	nets := make([]*network, len(list), len(list))
	for i := range keys {
		conf := &Config{
			List:      list,
			Threshold: threshold,
			Timeout:   timeout,
		}
		nets[i] = newDkgNetwork(gws[i], keys[i], conf, cb)
	}
	return nets
}

func stopnetworks(nets []*network) {
	for i := range nets {
		//fmt.Printf("Stopping gateway %p\n", &nets[i].gw)
		if err := nets[i].gw.Stop(); err != nil {
			panic(err)
		}
	}
}

func TestDKG(t *testing.T) {
	n := 5
	thr := n/2 + 1
	privs, gws := test.Gateways(n)
	//slog.Level = slog.LevelDebug
	//defer func() { slog.Level = slog.LevelPrint }()

	// waits for receiving n shares
	var wg sync.WaitGroup
	wg.Add(n)
	//var i = 1
	callback := func() {
		//fmt.Printf("callback called %d times...\n", i)
		wg.Done()
		//i++
	}
	nets := networks(privs, gws, thr, callback, 100*time.Millisecond)
	defer stopnetworks(nets)

	nets[0].dkg.Start()
	//fmt.Println("wg.Wait()...")
	wg.Wait()
	//fmt.Println("wg.Wait()... DONE")

}
