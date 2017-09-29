package groupsig

// types

// Pop --
type Pop Signature

// Proof-of-Possesion

// GeneratePop --
func GeneratePop(sec Seckey, pub Pubkey) Pop {
	return Pop(Sign(sec, pub.Serialize()))
}

// Verification

// VerifyPop --
func VerifyPop(pub Pubkey, pop Pop) bool {
	return VerifySig(pub, pub.Serialize(), Signature(pop))
}
