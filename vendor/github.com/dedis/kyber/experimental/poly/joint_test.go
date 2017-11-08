// +build experimental

package poly

import (
	"bytes"
	"fmt"
	"testing"

	_ "github.com/dedis/kyber/abstract"
)

/////// TESTING ///////

func TestReceiverAddDeal(t *testing.T) {
	dealers, receivers := generateNDealerMReceiver(Threshold{3, 3, 4}, 3, 4)
	// Test adding one dealer
	_, e1 := receivers[0].AddDeal(0, dealers[0])
	if e1 != nil {
		t.Error(fmt.Sprintf("AddDeal should not return an error : %v", e1))
	}

	// Test adding another dealer with same index
	_, e2 := receivers[0].AddDeal(0, dealers[1])
	if e2 != nil {
		t.Error(fmt.Sprintf("AddDeal should not return an error : %v", e2))
	}

	// Test adding another dealer with different index !
	_, e3 := receivers[0].AddDeal(1, dealers[2])
	if e3 == nil {
		t.Error(fmt.Sprintf("AddDeal should have returned an error (adding dealer to a different index for same receiver)"))
	}
}

// Test the AddReponse func
func rightDealerAddResponse(t *testing.T) {
	// Test if all goes well with the right inputs
	n := 3
	m := 4
	dealers, receivers := generateNDealerMReceiver(Threshold{3, 3, 4}, n, m)
	states := make([]*State, len(dealers))
	for i := 0; i < len(dealers); i++ {
		states[i] = new(State).Init(*dealers[i])
	}
	// for each receiver
	for i := 0; i < m; i++ {
		// add all the dealers
		for j := 0; j < n; j++ {
			resp, err := receivers[i].AddDeal(i, dealers[j])
			if err != nil {
				t.Error("AddDeal should not generate error")
			}
			// then give the response back to the dealer
			err = states[j].AddResponse(i, resp)
			if err != nil {
				t.Error(fmt.Sprintf("AddResponse should not generate any error : %v", err))
			}
		}
	}
	for j := 0; j < n; j++ {
		val := states[j].DealCertified()
		if val != nil {
			t.Error(fmt.Sprintf("Dealer %d should be certified : %v", j, val))
		}
	}

}
func TestDealerAddResponse(t *testing.T) {
	rightDealerAddResponse(t)
	wrongDealerAddResponse(t)
}

// Test the AddReponse func with wrong inputs
func wrongDealerAddResponse(t *testing.T) {
	n := 3
	m := 4
	dealers, receivers := generateNDealerMReceiver(Threshold{3, 3, 4}, n, m)
	r1, _ := receivers[0].AddDeal(0, dealers[0])
	state := new(State).Init(*dealers[0])
	err := state.AddResponse(1, r1)
	if err == nil {
		t.Error("AddResponse should have returned an error when given the wrong index share")
	}
}

func TestProduceSharedSecret(t *testing.T) {
	T := 4
	m := 5
	_, receivers := generateNMSetup(Threshold{T, m, m}, T, m)
	s1, err := receivers[0].ProduceSharedSecret()
	if err != nil {
		t.Error(fmt.Sprintf("ProduceSharedSecret should not gen any error : %v", err))
	}
	s2, err := receivers[1].ProduceSharedSecret()
	if err != nil {
		t.Error(fmt.Sprintf("ProdueSharedSecret should not gen any error : %v", err))
	}

	if !s1.Pub.Equal(s2.Pub) {
		t.Error("SharedSecret's polynomials should be equals")
	}

	if v := s1.Pub.Check(receivers[1].index, *s2.Share); v == false {
		t.Error("SharedSecret's share can not be verified using another's receiver pubpoly")
	}
	if v := s2.Pub.Check(receivers[0].index, *s1.Share); v == false {
		t.Error("SharedSecret's share can not be verified using another's receiver pubpoly")
	}
}

func TestPolyInfoMarshalling(t *testing.T) {
	pl := Threshold{
		T: 3,
		R: 5,
		N: 8,
	}
	b := new(bytes.Buffer)
	err := testSuite.Write(b, &pl)
	if err != nil {
		t.Error(fmt.Sprintf("PolyInfo MarshalBinary should not return error : %v", err))
	}
	pl2 := Threshold{}
	err = testSuite.Read(bytes.NewBuffer(b.Bytes()), &pl2)
	if err != nil {
		t.Error(fmt.Sprintf("PolyInfo UnmarshalBinary should not return error : %v", err))
	}

	if !pl.Equal(pl2) {
		t.Error(fmt.Sprintf("PolyInfo's should be equals: \npl1 : %+v\npl2 : %+v", pl, pl2))
	}

}

func TestProduceSharedSecretMarshalledDealer(t *testing.T) {
	// Test if all goes well with the right inputs
	n := 3
	m := 3
	pl := Threshold{2, 3, 3}
	dealers, receivers := generateNDealerMReceiver(pl, n, m)
	// for each receiver
	for i := 0; i < m; i++ {
		// add all the dealers
		for j := 0; j < n; j++ {
			b := new(bytes.Buffer)
			err := testSuite.Write(b, dealers[j])
			if err != nil {
				t.Error("Write(Dealer) should not gen any error : ", err)
			}
			buf := b.Bytes()
			bb := bytes.NewBuffer(buf)
			d2 := new(Deal).UnmarshalInit(pl.T, pl.R, pl.N, testSuite)
			err = testSuite.Read(bb, d2)
			if err != nil {
				t.Error("Read(Dealer) should not gen any error : ", err)
			}
			receivers[i].AddDeal(i, d2)
		}
	}
	_, err := receivers[0].ProduceSharedSecret()
	if err != nil {
		t.Error(fmt.Sprintf("ProduceSharedSecret with Marshalled dealer should work : %v", err))
	}
}
