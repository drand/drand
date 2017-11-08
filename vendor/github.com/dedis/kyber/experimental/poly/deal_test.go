// +build experimental

package poly

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/dedis/kyber/abstract"
	"github.com/dedis/kyber/anon"
	"github.com/dedis/kyber/config"
	"github.com/dedis/kyber/edwards"
	"github.com/dedis/kyber/nist"
	"github.com/dedis/kyber/random"
)

var suite = nist.NewAES128SHA256P256()
var altSuite = edwards.NewAES128SHA256Ed25519(false)

var secretKey = produceKeyPair()
var DealerKey = produceKeyPair()

var pt = 10
var r = 15
var numInsurers = 20

var insurerKeys = produceinsurerKeys()
var insurerList = produceinsurerList()

var basicDeal = new(Deal).ConstructDeal(secretKey, DealerKey, pt, r, insurerList)
var basicState = new(State).Init(*basicDeal)

func produceKeyPair() *config.KeyPair {
	keyPair := new(config.KeyPair)
	keyPair.Gen(suite, random.Stream)
	return keyPair
}

func produceAltKeyPair() *config.KeyPair {
	keyPair := new(config.KeyPair)
	keyPair.Gen(altSuite, random.Stream)
	return keyPair
}

func produceinsurerKeys() []*config.KeyPair {
	newArray := make([]*config.KeyPair, numInsurers, numInsurers)
	for i := 0; i < numInsurers; i++ {
		newArray[i] = produceKeyPair()
	}
	return newArray
}

func produceinsurerList() []abstract.Point {
	newArray := make([]abstract.Point, numInsurers, numInsurers)
	for i := 0; i < numInsurers; i++ {
		newArray[i] = insurerKeys[i].Public
	}
	return newArray
}

// Tests that check whether a method panics can use this funcition
func recoverTest(t *testing.T, message string) {
	if r := recover(); r == nil {
		t.Error(message)
	}
}

// Verifies that Init properly initalizes a new signature object
func TestDealSignatureInit(t *testing.T) {
	sig := []byte("This is a test signature")
	p := new(signature).init(suite, sig)
	if p.suite != suite {
		t.Error("Suite not properly initialized.")
	}
	if !reflect.DeepEqual(sig, p.signature) {
		t.Error("Signature not properly initialized.")
	}
}

// Verifies that UnMarshalInit properly initalizes for unmarshalling
func TestDealSignatureUnMarshalInit(t *testing.T) {
	p := new(signature).UnmarshalInit(suite)
	if p.suite != suite {
		t.Error("Suite not properly initialized.")
	}
}

// Verifies that signature's marshalling code works
func TestDealSignatureBinaryMarshalling(t *testing.T) {
	// Tests BinaryMarshal, BinaryUnmarshal, and MarshalSize
	sig := basicDeal.sign(numInsurers-1, insurerKeys[numInsurers-1], sigMsg)
	encodedSig, err := sig.MarshalBinary()
	if err != nil || len(encodedSig) != sig.MarshalSize() {
		t.Fatal("Marshalling failed: ", err,
			len(encodedSig) != sig.MarshalSize())
	}

	decodedSig := new(signature).UnmarshalInit(suite)
	err = decodedSig.UnmarshalBinary(encodedSig)
	if err != nil {
		t.Fatal("UnMarshalling failed: ", err)
	}
	if !sig.Equal(decodedSig) {
		t.Error("Decoded signature not equal to original")
	}
	if basicDeal.verifySignature(numInsurers-1, decodedSig, sigMsg) != nil {
		t.Error("Decoded signature failed to be verified.")
	}

	// Tests MarshlTo and UnmarshalFrom
	sig2 := basicDeal.sign(1, insurerKeys[1], sigMsg)
	bufWriter := new(bytes.Buffer)
	bytesWritter, errs := sig2.MarshalTo(bufWriter)
	if bytesWritter != sig2.MarshalSize() || errs != nil {
		t.Fatal("MarshalTo failed: ", bytesWritter, err)
	}

	decodedSig2 := new(signature).UnmarshalInit(suite)
	bufReader := bytes.NewReader(bufWriter.Bytes())
	bytesRead, errs2 := decodedSig2.UnmarshalFrom(bufReader)
	if bytesRead != sig2.MarshalSize() || errs2 != nil {
		t.Fatal("UnmarshalFrom failed: ", bytesRead, errs2)
	}
	if sig2.MarshalSize() != decodedSig2.MarshalSize() {
		t.Error("MarshalSize of decoded and original differ: ",
			sig2.MarshalSize(), decodedSig2.MarshalSize())
	}
	if !sig2.Equal(decodedSig2) {
		t.Error("signature read does not equal original")
	}
	if basicDeal.verifySignature(1, decodedSig2, sigMsg) != nil {
		t.Error("Read signature failed to be verified.")
	}

}

// Verifies that Equal properly works for signature objects
func TestDealSignatureEqual(t *testing.T) {
	sig := []byte("This is a test")
	p := new(signature).init(suite, sig)
	if !p.Equal(p) {
		t.Error("signature should equal itself.")
	}

	// Error cases
	p2 := new(signature).init(nil, sig)
	if p.Equal(p2) {
		t.Error("signature's differ in suite.")
	}
	p2 = new(signature).init(suite, nil)
	if p.Equal(p2) {
		t.Error("signature's differ in signature.")
	}
}

// Verifies that Init properly initalizes a new blameProof object
func TestBlameProofInit(t *testing.T) {
	proof := []byte("This is a test")
	sig := []byte("This too is a test")
	p := new(signature).init(suite, sig)
	bp := new(blameProof).init(suite, DealerKey.Public, proof, p)
	if suite != bp.suite {
		t.Error("Suite not properly initialized.")
	}
	if !bp.diffieKey.Equal(DealerKey.Public) {
		t.Error("Diffie-Hellman key not properly initialized.")
	}
	if !reflect.DeepEqual(bp.proof, proof) {
		t.Error("Diffie-Hellman proof not properly initialized.")
	}
	if !p.Equal(&bp.signature) {
		t.Error("PromisSignature not properly initialized.")
	}
}

// Verifies that UnMarshalInit properly initalizes for unmarshalling
func TestBlameProofUnMarshalInit(t *testing.T) {
	bp := new(blameProof).UnmarshalInit(suite)
	if bp.suite != suite {
		t.Error("blameProof not properly initialized.")
	}
}

// Verifies that Equal properly works for signature objects
func TestBlameProofEqual(t *testing.T) {
	p := new(signature).init(suite, []byte("Test"))
	bp := new(blameProof).init(suite, DealerKey.Public, []byte("Test"), p)
	if !bp.Equal(bp) {
		t.Error("blameProof should equal itself.")
	}

	// Error cases
	bp2 := new(blameProof).init(nil, DealerKey.Public, []byte("Test"), p)
	if bp.Equal(bp2) {
		t.Error("blameProof differ in key suites.")
	}
	bp2 = new(blameProof).init(suite, suite.Point().Base(), []byte("Test"), p)
	if bp.Equal(bp2) {
		t.Error("blameProof differ in diffie-keys.")
	}
	bp2 = new(blameProof).init(suite, DealerKey.Public, []byte("Differ"), p)
	if bp.Equal(bp2) {
		t.Error("blameProof differ in hash proof.")
	}
	p2 := new(signature).init(suite, []byte("Differ"))
	bp2 = new(blameProof).init(suite, DealerKey.Public, []byte("Test"), p2)
	if bp.Equal(bp2) {
		t.Error("blameProof differ in signatures.")
	}
}

// Verifies that blameProof's marshalling methods work properly.
func TestBlameProofBinaryMarshalling(t *testing.T) {
	// Create a bad dealobject. That a blame proof would succeed.
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt, r, insurerList)
	badKey := insurerKeys[numInsurers-1]
	diffieBase := deal.suite.Point().Mul(DealerKey.Public, badKey.Secret)
	diffieSecret := deal.diffieHellmanSecret(diffieBase)
	badShare := deal.suite.Scalar().Add(badKey.Secret, diffieSecret)
	deal.secrets[0] = badShare

	// Tests BinaryMarshal, BinaryUnmarshal, and MarshalSize
	bp, _ := deal.blame(0, insurerKeys[0])
	encodedBp, err := bp.MarshalBinary()
	if err != nil || len(encodedBp) != bp.MarshalSize() {
		t.Fatal("Marshalling failed: ", err)
	}

	decodedBp := new(blameProof).UnmarshalInit(suite)
	err = decodedBp.UnmarshalBinary(encodedBp)
	if err != nil {
		t.Fatal("UnMarshalling failed: ", err)
	}
	if !bp.Equal(decodedBp) {
		t.Error("Decoded blameProof not equal to original")
	}
	if bp.MarshalSize() != decodedBp.MarshalSize() {
		t.Error("MarshalSize of decoded and original differ: ",
			bp.MarshalSize(), decodedBp.MarshalSize())
	}
	if deal.verifyBlame(0, decodedBp) != nil {
		t.Error("Decoded blameProof failed to be verified.")
	}

	// Tests MarshlTo and UnmarshalFrom
	bp2, _ := basicDeal.blame(0, insurerKeys[0])
	bufWriter := new(bytes.Buffer)
	bytesWritter, errs := bp2.MarshalTo(bufWriter)
	if bytesWritter != bp2.MarshalSize() || errs != nil {
		t.Fatal("MarshalTo failed: ", bytesWritter, err)
	}

	decodedBp2 := new(blameProof).UnmarshalInit(suite)
	bufReader := bytes.NewReader(bufWriter.Bytes())
	bytesRead, errs2 := decodedBp2.UnmarshalFrom(bufReader)
	if bytesRead != bp2.MarshalSize() || errs2 != nil {
		t.Fatal("UnmarshalFrom failed: ", bytesRead, errs2)
	}
	if bp2.MarshalSize() != decodedBp2.MarshalSize() {
		t.Error("MarshalSize of decoded and original differ: ",
			bp2.MarshalSize(), decodedBp2.MarshalSize())
	}
	if !bp2.Equal(decodedBp2) {
		t.Error("blameProof read does not equal original")
	}
	if deal.verifyBlame(0, decodedBp2) != nil {
		t.Error("Decoded blameProof failed to be verified.")
	}

}

// Verifies that constructSignatureResponse properly initalizes a new Response
func TestResponseConstructSignatureResponse(t *testing.T) {
	sig := basicDeal.sign(0, insurerKeys[0], sigMsg)

	response := new(Response).constructSignatureResponse(sig)
	if response.rtype != signatureResponse {
		t.Error("Response type not properly initialized.")
	}
	if !sig.Equal(response.signature) {
		t.Error("Signature not properly initialized.")
	}
}

// Verifies that constructBlameProofResponse properly initalizes a new Response
func TestResponseConstructProofResponse(t *testing.T) {
	proof, _ := basicDeal.blame(0, insurerKeys[0])

	response := new(Response).constructBlameProofResponse(proof)
	if response.rtype != blameProofResponse {
		t.Error("Response type not properly initialized.")
	}
	if !proof.Equal(response.blameProof) {
		t.Error("Proof not properly initialized.")
	}
}

// Verifies that UnMarshalInit properly initalizes for unmarshalling
func TestResponseUnMarshalInit(t *testing.T) {
	response := new(Response).UnmarshalInit(suite)
	if response.suite != suite {
		t.Error("Response not properly initialized.")
	}
}

// Verifies that Equal properly works for Response objects
func TestResponseEqual(t *testing.T) {
	sig := basicDeal.sign(0, insurerKeys[0], sigMsg)
	proof, _ := basicDeal.blame(0, insurerKeys[0])

	response := new(Response).constructBlameProofResponse(proof)
	if !response.Equal(response) {
		t.Error("Response should equal itself.")
	}

	// Error cases
	response2 := new(Response).constructSignatureResponse(sig)
	if response.Equal(response2) {
		t.Error("Response differ in type.")
	}
	response2 = new(Response).constructBlameProofResponse(proof)
	response2.blameProof, _ = basicDeal.blame(1, insurerKeys[1])
	if response.Equal(response2) {
		t.Error("Response differ in Proof.")
	}
	response = new(Response).constructSignatureResponse(sig)
	response2 = new(Response).constructSignatureResponse(sig)
	response2.signature = basicDeal.sign(1, insurerKeys[1], sigMsg)
	if response.Equal(response2) {
		t.Error("Response differ in Signatures.")
	}

	// Verify that equal panics if the messages are uninitialized
	test := func() {
		defer recoverTest(t, "Equal should have panicked.")
		new(Response).Equal(new(Response))
	}
	test()
}

func responseMarshallingHelper(t *testing.T, response *Response) {

	// Tests BinaryMarshal, BinaryUnmarshal, and MarshalSize
	encodedResponse, err := response.MarshalBinary()
	if err != nil || len(encodedResponse) != response.MarshalSize() {
		t.Fatal("Marshalling failed: ", err)
	}

	decodedResponse := new(Response).UnmarshalInit(suite)
	err = decodedResponse.UnmarshalBinary(encodedResponse)
	if err != nil {
		t.Fatal("UnMarshalling failed: ", err)
	}
	if !response.Equal(decodedResponse) {
		t.Error("Decoded blameProof not equal to original")
	}
	if response.MarshalSize() != decodedResponse.MarshalSize() {
		t.Error("MarshalSize of decoded and original differ: ",
			response.MarshalSize(), decodedResponse.MarshalSize())
	}

	// Tests MarshlTo and UnmarshalFrom
	bufWriter := new(bytes.Buffer)
	bytesWritter, errs := response.MarshalTo(bufWriter)
	if bytesWritter != response.MarshalSize() || errs != nil {
		t.Fatal("MarshalTo failed: ", bytesWritter, err)
	}

	decodedResponse = new(Response).UnmarshalInit(suite)
	bufReader := bytes.NewReader(bufWriter.Bytes())
	bytesRead, errs2 := decodedResponse.UnmarshalFrom(bufReader)
	if bytesRead != response.MarshalSize() || errs2 != nil {
		t.Fatal("UnmarshalFrom failed: ", bytesRead, errs2)
	}
	if response.MarshalSize() != decodedResponse.MarshalSize() {
		t.Error("MarshalSize of decoded and original differ: ",
			response.MarshalSize(), decodedResponse.MarshalSize())
	}
	if !response.Equal(decodedResponse) {
		t.Error("Response read does not equal original")
	}
}

// Verifies that Response's marshalling methods work properly.
func TestResponseBinaryMarshalling(t *testing.T) {

	// Verify a signature response can be encoded properly
	sig := basicDeal.sign(0, insurerKeys[0], sigMsg)
	response := new(Response).constructSignatureResponse(sig)
	responseMarshallingHelper(t, response)

	// Verify a proof response can be encoded properly
	proof, _ := basicDeal.blame(0, insurerKeys[0])
	response = new(Response).constructBlameProofResponse(proof)
	responseMarshallingHelper(t, response)
}

// Verifies that Constructdealproperly initalizes a new dealstruct
func TestDealConstructDeal(t *testing.T) {
	// Verify that a dealcan be initialized properly.
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)

	if !deal.id.Equal(secretKey.Public) {
		t.Error("id not initialized properly")
	}
	if DealerKey.Suite != deal.suite {
		t.Error("suite not initialized properly")
	}
	if secretKey.Suite != deal.suite {
		t.Error("suite not initialized properly")
	}
	if deal.t != pt {
		t.Error("t not initialized properly")
	}
	if deal.r != r {
		t.Error("r not initialized properly")
	}
	if deal.n != numInsurers {
		t.Error("n not initialized properly")
	}
	if !deal.pubKey.Equal(DealerKey.Public) {
		t.Error("Public Key not initialized properly")
	}
	if len(deal.secrets) != numInsurers {
		t.Error("Secrets array not initialized properly")
	}
	for i := 0; i < deal.n; i++ {
		if !insurerList[i].Equal(deal.insurers[i]) {
			t.Error("Public key for insurer not added:", i)
		}
		diffieBase := deal.suite.Point().Mul(insurerList[i],
			DealerKey.Secret)
		diffieSecret := deal.diffieHellmanSecret(diffieBase)
		share := deal.suite.Scalar().Sub(deal.secrets[i], diffieSecret)
		if !deal.pubPoly.Check(i, share) {
			t.Error("Polynomial Check failed for share ", i)
		}
	}

	// Error handling
	// First, verify that dealcreates its own copy of the array data.
	deal.insurers[0] = nil
	if insurerList[0] == nil {
		t.Error("Changing the return result shouldn't change the original array")
	}

	// Check that Constructdealpanics if n < t
	test := func() {
		defer recoverTest(t, "Constructdealshould have panicked.")
		new(Deal).ConstructDeal(secretKey, DealerKey, 2, r,
			[]abstract.Point{DealerKey.Public})
	}
	test()

	// Check that r is reset properly when r < t.
	test = func() {
		defer recoverTest(t, "Constructdealshould have panicked.")
		new(Deal).ConstructDeal(secretKey, DealerKey, pt, pt-1,
			insurerList)
	}
	test()

	// Check that r is reset properly when r > n.
	test = func() {
		defer recoverTest(t, "Constructdealshould have panicked.")
		new(Deal).ConstructDeal(secretKey, DealerKey, pt, numInsurers+1,
			insurerList)
	}
	test()

	// Check that Constructdealpanics if the keys are of different suites
	test = func() {
		defer recoverTest(t, "Constructdealshould have panicked.")
		new(Deal).ConstructDeal(produceAltKeyPair(), DealerKey, pt, r,
			insurerList)
	}
	test()
}

// Verifies that UnMarshalInit properly initalizes for unmarshalling
func TestDealUnMarshalInit(t *testing.T) {
	p := new(Deal).UnmarshalInit(pt, r, numInsurers, suite)
	if p.t != pt {
		t.Error("t not properly initialized.")
	}
	if p.r != r {
		t.Error("r not properly initialized.")
	}
	if p.n != numInsurers {
		t.Error("n not properly initialized.")
	}
	if p.suite != suite {
		t.Error("Suite not properly initialized.")
	}
}

// Tests that DealVerify properly rules out invalidly constructed Deal's
func TestDealverifyDeal(t *testing.T) {
	if basicDeal.verifyDeal() != nil {
		t.Error("dealis valid")
	}

	// Error handling
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.t = deal.n + 1
	if deal.verifyDeal() == nil {
		t.Error("dealis invalid: t > n")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.t = deal.r + 1
	if deal.verifyDeal() == nil {
		t.Error("dealis invalid: t > r")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.r = deal.n + 1
	if deal.verifyDeal() == nil {
		t.Error("dealis invalid: n > r")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.insurers = []abstract.Point{}
	if deal.verifyDeal() == nil {
		t.Error("dealis invalid: insurers list is the wrong length")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = []abstract.Scalar{}
	if deal.verifyDeal() == nil {
		t.Error("dealis invalid: secrets list is the wrong length")
	}
}

// Verifies that Id returns the id expected
func TestDealId(t *testing.T) {
	if basicDeal.Id() != secretKey.Public.String() {
		t.Error("Wrong id returned.")
	}
}

// Verifies that DealerId returns the id expected
func TestDealDealerId(t *testing.T) {
	if basicDeal.DealerId() != DealerKey.Public.String() {
		t.Error("Wrong id returned.")
	}
}

// Verifies that DealerKey returns a copy of the Dealer's long term key
func TestDealerKey(t *testing.T) {
	result := basicDeal.DealerKey()
	if !result.Equal(basicDeal.pubKey) &&
		result.String() != basicDeal.pubKey.String() {
		t.Fatal("Keys should be equal")
	}

	result.Base()
	if result.Equal(basicDeal.pubKey) {
		t.Error("Changing the return result shouldn't change the original key")
	}
}

// Verifies that Insurers returns the insurers slice expected
func TestDealInsurers(t *testing.T) {
	result := basicDeal.Insurers()
	for i := 0; i < basicDeal.n; i++ {
		if result[i] != basicDeal.insurers[i] {
			t.Fatal("Wrong insurers list returned.")
		}
	}

	result = basicDeal.Insurers()
	result[0] = nil
	if basicDeal.insurers[0] == nil {
		t.Error("Changing the return result shouldn't change the original array")
	}

}

// Tests that encrypting a secret with a diffie-hellman shared secret and then
// decrypting it succeeds.
func TestDealDiffieHellmanEncryptDecrypt(t *testing.T) {
	// key2 and DealerKey will be the two parties. The secret they are
	// sharing is the private key of secretKey
	key2 := produceKeyPair()

	diffieBaseBasic := basicDeal.suite.Point().Mul(key2.Public,
		DealerKey.Secret)
	diffieSecret := basicDeal.diffieHellmanSecret(diffieBaseBasic)
	encryptedSecret := basicDeal.suite.Scalar().Add(secretKey.Secret, diffieSecret)

	diffieBaseKey2 := basicDeal.suite.Point().Mul(DealerKey.Public,
		key2.Secret)
	diffieSecret = basicDeal.diffieHellmanSecret(diffieBaseKey2)
	secret := basicDeal.suite.Scalar().Sub(encryptedSecret, diffieSecret)

	if !secret.Equal(secretKey.Secret) {
		t.Error("Diffie-Hellman encryption/decryption failed.")
	}
}

// Tests that insurers can properly verify their shares. Makes sure that
// verification fails if the proper credentials are not supplied (aka Diffie-
// Hellman decryption failed).
func TestDealVerifyShare(t *testing.T) {
	if basicDeal.verifyShare(0, insurerKeys[0]) != nil {
		t.Error("The share should have been verified")
	}

	// Error handling
	if basicDeal.verifyShare(-1, insurerKeys[0]) == nil {
		t.Error("The share should not have been valid. Index is negative.")
	}
	if basicDeal.verifyShare(basicDeal.n, insurerKeys[0]) == nil {
		t.Error("The share should not have been valid. Index >= n")
	}
	if basicDeal.verifyShare(numInsurers-1, insurerKeys[0]) == nil {
		t.Error("Share should be invalid. Index and Public Key did not match.")
	}
}

// Verify that the dealcan produce a valid signature and then verify it.
// In short, all signatures produced by the sign method should be accepted.
func TestDealSignAndVerify(t *testing.T) {
	sig := basicDeal.sign(0, insurerKeys[0], sigMsg)
	if basicDeal.verifySignature(0, sig, sigMsg) != nil {
		t.Error("Signature failed to be validated")
	}
}

// Produces a bad signature that has a malformed approve message
func produceSigWithBadMessage() *signature {
	set := anon.Set{insurerKeys[0].Public}
	approveMsg := "Bad message"
	digSig := anon.Sign(insurerKeys[0].Suite, random.Stream, []byte(approveMsg),
		set, nil, 0, insurerKeys[0].Secret)
	return new(signature).init(insurerKeys[0].Suite, digSig)
}

// Verify that mallformed signatures are not accepted.
func TestDealVerifySignature(t *testing.T) {
	// Fail if the signature is not the specially formatted approve message.
	if basicDeal.verifySignature(0, produceSigWithBadMessage(), sigMsg) == nil {
		t.Error("Signature has a bad message and should be rejected.")
	}

	//Error Handling
	// Fail if a valid signature is applied to the wrong share.
	sig := basicDeal.sign(0, insurerKeys[0], sigMsg)
	if basicDeal.verifySignature(numInsurers-1, sig, sigMsg) == nil {
		t.Error("Signature is for the wrong share.")
	}
	// Fail if index is negative
	if basicDeal.verifySignature(-1, sig, sigMsg) == nil {
		t.Error("Error: Index < 0")
	}
	// Fail if index >= n
	if basicDeal.verifySignature(basicDeal.n, sig, sigMsg) == nil {
		t.Error("Error: Index >= n")
	}
	// Should return false if passed nil
	sig.signature = nil
	if basicDeal.verifySignature(0, sig, sigMsg) == nil {
		t.Error("Error: Signature is nil")
	}
}

// Verify that insurer secret shares can be revealed properly and verified.
func TestDealerevealShareAndShareVerify(t *testing.T) {
	DealShare := basicDeal.RevealShare(0, insurerKeys[0])
	if basicDeal.VerifyRevealedShare(0, DealShare) != nil {
		t.Error("The share should have been marked as valid")
	}

	// Error Handling
	if basicDeal.VerifyRevealedShare(-1, DealShare) == nil {
		t.Error("The index provided is too low.")
	}
	if basicDeal.VerifyRevealedShare(numInsurers, DealShare) == nil {
		t.Error("The index provided is too high.")
	}
	// Ensures the public polynomial fails when the share provided doesn't
	// match the index.
	if basicDeal.VerifyRevealedShare(2, DealShare) == nil {
		t.Error("The share provided is not for the index.")
	}
}

// Verify that insurers can properly create and verify blame proofs
func TestDealBlameAndVerify(t *testing.T) {

	// Create a bad dealobject. Create a new secret that will fail the
	// the public polynomial check.
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	badKey := insurerKeys[numInsurers-1]
	diffieBase := deal.suite.Point().Mul(DealerKey.Public,
		badKey.Secret)
	diffieSecret := deal.diffieHellmanSecret(diffieBase)
	badShare := deal.suite.Scalar().Add(badKey.Secret, diffieSecret)
	deal.secrets[0] = badShare

	validProof, err := deal.blame(0, insurerKeys[0])
	if err != nil {
		t.Fatal("Blame failed to be properly constructed")
	}
	if deal.verifyBlame(0, validProof) != nil {
		t.Error("The proof is valid and should be accepted.")
	}

	// Error handling
	if deal.verifyBlame(-10, validProof) == nil {
		t.Error("The i index is below 0")
	}
	if deal.verifyBlame(numInsurers, validProof) == nil {
		t.Error("The i index is at or above n")
	}

	goodDealShare, _ := basicDeal.blame(0, insurerKeys[0])
	if basicDeal.verifyBlame(0, goodDealShare) == nil {
		t.Error("Invalid blame: the share is actually good.")
	}
	badProof, _ := basicDeal.blame(0, insurerKeys[0])
	badProof.proof = []byte("Invalid zero-knowledge proof")
	if basicDeal.verifyBlame(0, badProof) == nil {
		t.Error("Invalid blame. Bad Diffie-Hellman key proof.")
	}
	badSignature, _ := basicDeal.blame(0, insurerKeys[0])
	badSignature.signature = *deal.sign(1, insurerKeys[1], sigMsg)
	if basicDeal.verifyBlame(0, badSignature) == nil {
		t.Error("Invalid blame. The signature is bad.")
	}
}

// Verify that insurers can properly produce responses
func TestDealProduceResponse(t *testing.T) {

	// Verify a valid signatureResponse can be created
	response, err := basicDeal.ProduceResponse(0, insurerKeys[0])
	if err != nil {
		t.Fatal("ProduceResponse should have succeeded")
	}
	if response.rtype != signatureResponse {
		t.Fatal("Response should be a blameProof")
	}
	if basicDeal.verifySignature(0, response.signature, sigMsg) != nil {
		t.Error("The proof is valid and should be accepted.")
	}

	// Verify a proper blameProofResponse can be created
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	badKey := insurerKeys[numInsurers-1]
	diffieBase := deal.suite.Point().Mul(DealerKey.Public,
		badKey.Secret)
	diffieSecret := deal.diffieHellmanSecret(diffieBase)
	badShare := deal.suite.Scalar().Add(badKey.Secret, diffieSecret)
	deal.secrets[0] = badShare

	response, err = deal.ProduceResponse(0, insurerKeys[0])
	if err != nil {
		t.Fatal("ProduceResponse should have succeeded")
	}
	if response.rtype != blameProofResponse {
		t.Fatal("Response should be a blameProof")
	}
	if deal.verifyBlame(0, response.blameProof) != nil {
		t.Error("The proof is valid and should be accepted.")
	}
}

// Verifies that Equal properly works for dealstructs
func TestDealEqual(t *testing.T) {
	// Make sure dealequals basicDealto make the error cases
	// below valid (if dealnever equals basicDeal, error cases are
	// trivially true). Secrets and the public polynomial must be set
	// equal in each case to make sure that dealand basicDealare
	// equal.
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	if !basicDeal.Equal(deal) {
		t.Error("Deals should be equal.")
	}

	// Error cases
	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	deal.id = DealerKey.Public // <--- should be secretKey.Public
	if basicDeal.Equal(deal) {
		t.Error("The id's are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	deal.suite = nil
	if basicDeal.Equal(deal) {
		t.Error("The suite's are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	deal.n = 0
	if basicDeal.Equal(deal) {
		t.Error("The n's are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	deal.t = 0
	if basicDeal.Equal(deal) {
		t.Error("The t's are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	deal.r = 0
	if basicDeal.Equal(deal) {
		t.Error("The r's are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	deal.pubKey = suite.Point().Base()
	if basicDeal.Equal(deal) {
		t.Error("The public keys are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	if basicDeal.Equal(deal) {
		t.Error("The public polynomials are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.secrets = basicDeal.secrets
	deal.pubPoly = basicDeal.pubPoly
	deal.insurers = make([]abstract.Point, deal.n, deal.n)
	copy(deal.insurers, insurerList)
	deal.insurers[numInsurers-1] = suite.Point().Base()
	if basicDeal.Equal(deal) {
		t.Error("The insurers array are not equal")
	}

	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	deal.pubPoly = basicDeal.pubPoly
	if basicDeal.Equal(deal) {
		t.Error("The secrets array are not equal")
	}
}

// Verifies that Deal's marshalling functions work properly
func TestDealBinaryMarshalling(t *testing.T) {

	// Tests BinaryMarshal, BinaryUnmarshal, and MarshalSize
	encodedP, err := basicDeal.MarshalBinary()
	if err != nil || len(encodedP) != basicDeal.MarshalSize() {
		t.Fatal("Marshalling failed: ", err)
	}

	decodedP := new(Deal).UnmarshalInit(pt, r, numInsurers, suite)
	err = decodedP.UnmarshalBinary(encodedP)
	if err != nil {
		t.Fatal("UnMarshalling failed: ", err)
	}
	if !basicDeal.Equal(decodedP) {
		t.Error("Decoded dealnot equal to original")
	}

	// Tests MarshlTo and UnmarshalFrom
	bufWriter := new(bytes.Buffer)
	bytesWritter, errs := basicDeal.MarshalTo(bufWriter)

	if bytesWritter != basicDeal.MarshalSize() || errs != nil {
		t.Fatal("MarshalTo failed: ", bytesWritter, err)
	}

	decodedP2 := new(Deal).UnmarshalInit(pt, r, numInsurers, suite)
	bufReader := bytes.NewReader(bufWriter.Bytes())
	bytesRead, errs2 := decodedP2.UnmarshalFrom(bufReader)
	if bytesRead != decodedP2.MarshalSize() ||
		basicDeal.MarshalSize() != decodedP2.MarshalSize() ||
		errs2 != nil {
		t.Fatal("UnmarshalFrom failed: ", bytesRead, errs2)
	}
	if basicDeal.MarshalSize() != decodedP2.MarshalSize() {
		t.Error("MarshalSize's differ: ", basicDeal.MarshalSize(),
			decodedP2.MarshalSize())
	}
	if !basicDeal.Equal(decodedP2) {
		t.Error("dealread does not equal original")
	}

	// Verify that unmarshalling fails if the dealcreated is invalid.
	// In this case, the unmarshalling defaults are invalid.
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt,
		r, insurerList)
	encodedP, err = deal.MarshalBinary()
	if err != nil || len(encodedP) != basicDeal.MarshalSize() {
		t.Fatal("Marshalling failed: ", err)
	}

	decodedP = new(Deal).UnmarshalInit(pt, 1, numInsurers, suite)
	err = decodedP.UnmarshalBinary(encodedP)
	if err == nil {
		t.Fatal("UnMarshalling should have failed: ", err)
	}
}

// Verifies that Init properly initalizes a new State object
func TestStateInit(t *testing.T) {
	DealState := new(State).Init(*basicDeal)
	if !basicDeal.Equal(&DealState.Deal) {
		t.Error("dealnot properly initialized")
	}
	if len(DealState.responses) != numInsurers {
		t.Error("Responses array not properly initialized")
	}
}

// Verify that State can properly add signature and blame responses
func TestStateAddSignature(t *testing.T) {
	DealState := new(State).Init(*basicDeal)
	for i := 0; i < numInsurers; i++ {
		// Verify valid signatures are added.
		sig := DealState.Deal.sign(i, insurerKeys[i], sigMsg)
		response := new(Response).constructSignatureResponse(sig)
		err := DealState.AddResponse(i, response)
		if err != nil || !sig.Equal(DealState.responses[i].signature) {
			t.Error("Signature failed to be added", err)
		}

		err = DealState.AddResponse(i, response)
		if err == nil {
			t.Error("A particular response entry should be assigned only once")
		}
	}
	i := 0
	deal := new(Deal).ConstructDeal(secretKey, DealerKey, pt, r, insurerList)
	DealState = new(State).Init(*deal)

	// Verify invalid signatures are not added.
	sig := DealState.Deal.sign(i, insurerKeys[i], sigMsg)
	response := new(Response).constructSignatureResponse(sig)
	err := DealState.AddResponse(i+1, response)
	if err == nil || DealState.responses[i] != nil {
		t.Error("Signature is invalid and should not be added.", err)
	}

	// Change the response to an error and verify it is not added.
	response.rtype = errorResponse
	err = DealState.AddResponse(i, response)
	if err == nil || DealState.responses[i] != nil {
		t.Error("Signature is invalid and should not be added.", err)
	}

	// Verify invalid blameproofs are not added.
	bproof, _ := DealState.Deal.blame(i, insurerKeys[i])
	response = new(Response).constructBlameProofResponse(bproof)
	err = DealState.AddResponse(i, response)
	if err == nil || DealState.responses[i] != nil {
		t.Error("Invalid blameproof should not have been added.")
	}

	// Verify a valid blameproof can be added.
	DealState.Deal.secrets[i] = secretKey.Secret
	bproof, _ = DealState.Deal.blame(i, insurerKeys[i])
	response = new(Response).constructBlameProofResponse(bproof)
	err = DealState.AddResponse(i, response)
	if err != nil || !bproof.Equal(DealState.responses[i].blameProof) {
		t.Error("Valid blameproof should have been added.")
	}

}

// Verify State's DealCertify function
func TestStateDealCertified(t *testing.T) {
	deal := new(Deal).ConstructDeal(secretKey, DealerKey,
		pt, r, insurerList)
	DealState := new(State).Init(*deal)

	// Insure that bad blameProof structs do not cause the Deal
	// to be considered uncertified.
	bproof, _ := DealState.Deal.blame(0, insurerKeys[0])
	response := new(Response).constructBlameProofResponse(bproof)
	DealState.AddResponse(0, response)

	// Once enough signatures have been added, the dealshould remain
	// certified.
	for i := 1; i < numInsurers; i++ {
		sig := DealState.Deal.sign(i, insurerKeys[i], sigMsg)
		response := new(Response).constructSignatureResponse(sig)
		DealState.AddResponse(i, response)

		err := DealState.DealCertified()
		if i < r && err == nil {
			t.Error("Not enough signtures have been added yet", i, r)
		} else if i >= r && err != nil {
			t.Error("dealshould be valid now.")
			t.Error(DealState.DealCertified())
		}
	}

	// Error handling

	// If the dealfails verifyDeal, it should be uncertified even if
	// everything else is okay.
	DealState.Deal.n = 0
	if err := DealState.DealCertified(); err == nil {
		t.Error("The dealis malformed and should be uncertified")
	}

	// Make sure that one valid blameProof makes the dealforever
	// uncertified
	deal = new(Deal).ConstructDeal(secretKey, DealerKey, pt, r, insurerList)
	DealState = new(State).Init(*deal)
	DealState.Deal.secrets[0] = deal.suite.Scalar()
	bproof, _ = DealState.Deal.blame(0, insurerKeys[0])
	response = new(Response).constructBlameProofResponse(bproof)
	DealState.AddResponse(0, response)

	for i := 1; i < numInsurers; i++ {
		sig := DealState.Deal.sign(i, insurerKeys[i], sigMsg)
		response := new(Response).constructSignatureResponse(sig)
		DealState.AddResponse(i, response)
		if DealState.DealCertified() == nil {
			t.Error("A valid blameProof makes this uncertified")
		}
	}
}

// Verify State's SufficientSignatures function
func TestStateSufficientSignatures(t *testing.T) {
	deal := new(Deal).ConstructDeal(secretKey, DealerKey,
		pt, r, insurerList)
	DealState := new(State).Init(*deal)

	// Add a valid blameproof to the start. Ensure it doesn't affect
	// the results.
	DealState.Deal.secrets[0] = deal.suite.Scalar()
	bproof, _ := DealState.Deal.blame(0, insurerKeys[0])
	response := new(Response).constructBlameProofResponse(bproof)
	DealState.AddResponse(0, response)

	// Once enough signatures have been added, the dealshould remain
	// certified.
	for i := 1; i < numInsurers; i++ {
		sig := DealState.Deal.sign(i, insurerKeys[i], sigMsg)
		response := new(Response).constructSignatureResponse(sig)
		DealState.AddResponse(i, response)

		err := DealState.SufficientSignatures()
		if i < r && err == nil {
			t.Error("Not enough signtures have been added yet", i, r)
		} else if i >= r && err != nil {
			t.Error("dealshould be valid now.")
			t.Error(DealState.SufficientSignatures())
		}
	}
}

// Verify State's RevealShare function
func TestStateRevealShare(t *testing.T) {

	deal := new(Deal).ConstructDeal(secretKey, DealerKey,
		pt, r, insurerList)
	DealState := new(State).Init(*deal)

	test := func() {
		defer recoverTest(t, "RevealShare should have panicked.")
		DealState.RevealShare(0, insurerKeys[0])
	}
	test()

	// Add a valid blameproof to the start.
	DealState.Deal.secrets[0] = deal.suite.Scalar()
	bproof, _ := DealState.Deal.blame(0, insurerKeys[0])
	response := new(Response).constructBlameProofResponse(bproof)
	DealState.AddResponse(0, response)

	// Add enough signatures for the dealto be certified otherwise.
	for i := 1; i < r+1; i++ {
		sig := DealState.Deal.sign(i, insurerKeys[i], sigMsg)
		response := new(Response).constructSignatureResponse(sig)
		DealState.AddResponse(i, response)
	}

	// Verify that attempting to reveal a bad share results in an error.
	share, err := DealState.RevealShare(0, insurerKeys[0])
	if err == nil || share != nil {
		t.Error("No error should have been produced: ", err)
	}

	// Insure a good share can be revealed.
	share, err = DealState.RevealShare(1, insurerKeys[1])
	if err != nil {
		t.Error("No error should have been produced: ", err)
	}
	if err := DealState.Deal.VerifyRevealedShare(1, share); err != nil {
		t.Error("Share should be valid:", err)
	}
}

// Tests all the string functions. Simply calls them to make sure they return.
func TestString(t *testing.T) {
	sig := basicDeal.sign(0, insurerKeys[0], sigMsg)
	sig.String()

	bp, _ := basicDeal.blame(0, insurerKeys[0])
	bp.String()

	basicDeal.String()

	response := new(Response).constructSignatureResponse(sig)
	response.String()

	response = new(Response).constructBlameProofResponse(bp)
	response.String()
}

func TestDealAbstractEncoding(t *testing.T) {
	deal := new(Deal).ConstructDeal(secretKey, DealerKey,
		pt, r, insurerList)
	w := new(bytes.Buffer)
	err := suite.Write(w, deal)

	buf := w.Bytes()

	p := new(Deal).UnmarshalInit(pt, r, numInsurers, suite)
	r := bytes.NewBuffer(buf)
	err = suite.Read(r, p)
	if err != nil {
		t.Error("dealshould not gen any error while encoding")
	}
	if !deal.Equal(p) {
		t.Error("dealshould be equals")
	}
}
