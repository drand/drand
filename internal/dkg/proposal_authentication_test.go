package dkg

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/internal/util"
	drand "github.com/drand/drand/v2/protobuf/dkg"
)

const authTestBeaconID = "default"

func newKeyedParticipant(t *testing.T, name string) (*key.Pair, *drand.Participant) {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	pair, err := key.NewKeyPair(name+":443", sch)
	require.NoError(t, err)
	participant, err := util.PublicKeyAsParticipant(pair.Public)
	require.NoError(t, err)
	return pair, participant
}

// newPacketTestProcess returns a DKG process for the node owning pair, whose last finished DKG is finished, if any.
func newPacketTestProcess(t *testing.T, pair *key.Pair, finished *DBState) *Process {
	t.Helper()
	store, err := NewDKGStore(t.TempDir())
	require.NoError(t, err)
	if finished != nil {
		require.NoError(t, store.SaveFinished(authTestBeaconID, finished))
	}

	client := &MockDKGClient{}
	client.On("Packet", mock.Anything).Return(nil)
	process := NewDKGProcess(
		store,
		stubbedBeacon{kp: pair},
		util.NewFanOutChan[SharingOutput](),
		client,
		nil,
		Config{},
		log.New(nil, log.ErrorLevel, true),
	)
	t.Cleanup(process.Close)
	return process
}

// signedPacket signs packet over terms with the key of signer, on behalf of sender.
func signedPacket(t *testing.T, signer *key.Pair, sender string, packet *drand.GossipPacket, terms *drand.ProposalTerms) *drand.GossipPacket {
	t.Helper()
	sig, err := signer.Scheme().AuthScheme.Sign(signer.Key, messageForSigning(authTestBeaconID, packet, terms))
	require.NoError(t, err)
	packet.Metadata = &drand.GossipMetadata{BeaconID: authTestBeaconID, Address: sender, Signature: sig}
	return packet
}

func proposalPacket(terms *drand.ProposalTerms) *drand.GossipPacket {
	return &drand.GossipPacket{Packet: &drand.GossipPacket_Proposal{Proposal: terms}}
}

func abortPacket() *drand.GossipPacket {
	return &drand.GossipPacket{Packet: &drand.GossipPacket_Abort{Abort: &drand.AbortDKG{Reason: "none"}}}
}

func TestPacketRejectsProposalForgedForLeader(t *testing.T) {
	_, leader := newKeyedParticipant(t, "leader")
	memberPair, member := newKeyedParticipant(t, "member")
	victimPair, victim := newKeyedParticipant(t, "victim")
	outsiderPair, outsider := newKeyedParticipant(t, "outsider")
	freshPair, fresh := newKeyedParticipant(t, "fresh")

	tests := []struct {
		name string
		// the key listed under the address of the leader, and its owner
		forged *drand.Participant
		signer *key.Pair
		// the rest of the roster
		remaining []*drand.Participant
		joining   []*drand.Participant
		leaving   []*drand.Participant
		// leaderJoins lists the forged leader as a joiner instead of a remainer
		leaderJoins bool
		// expectedError is nil when the packet signature must be the reason of the rejection
		expectedError error
	}{
		{
			name:          "by a member listing its own key twice",
			forged:        member,
			signer:        memberPair,
			remaining:     []*drand.Participant{member, victim},
			expectedError: ErrDuplicateParticipant,
		},
		{
			name:      "by a member listing another key",
			forged:    fresh,
			signer:    freshPair,
			remaining: []*drand.Participant{member, victim},
		},
		{
			name:          "by a joiner listing its own key twice",
			forged:        outsider,
			signer:        outsiderPair,
			remaining:     []*drand.Participant{member, victim},
			joining:       []*drand.Participant{outsider},
			expectedError: ErrDuplicateParticipant,
		},
		{
			name:      "by a joiner listing another key",
			forged:    fresh,
			signer:    freshPair,
			remaining: []*drand.Participant{member, victim},
			joining:   []*drand.Participant{outsider},
		},
		{
			name:          "by listing the leader as a joiner with another key",
			forged:        fresh,
			signer:        freshPair,
			remaining:     []*drand.Participant{member, victim},
			leaderJoins:   true,
			expectedError: ErrLeaderCantJoinAfterFirstEpoch,
		},
		{
			name:          "by listing the leader as leaving and as a joiner with another key",
			forged:        fresh,
			signer:        freshPair,
			remaining:     []*drand.Participant{member, victim},
			leaving:       []*drand.Participant{leader},
			leaderJoins:   true,
			expectedError: ErrLeaderCantJoinAfterFirstEpoch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			finished := NewCompleteDKGEntry(t, authTestBeaconID, Complete, leader, member, victim)
			process := newPacketTestProcess(t, victimPair, finished)

			forgedLeader := withAddressOf(test.forged, leader)
			terms := NewValidProposal(authTestBeaconID, 2, forgedLeader)
			terms.Remaining = append(terms.Remaining, test.remaining...)
			terms.Joining = test.joining
			terms.Leaving = test.leaving
			if test.leaderJoins {
				terms.Remaining = test.remaining
				terms.Joining = []*drand.Participant{forgedLeader}
			}
			terms.Threshold = 3
			packet := signedPacket(t, test.signer, leader.Address, proposalPacket(terms), terms)

			_, err := process.Packet(context.Background(), packet)
			if test.expectedError != nil {
				require.ErrorIs(t, err, test.expectedError)
			} else {
				require.ErrorContains(t, err, "invalid packet signature")
				// the roster is rejected as well, whoever signs it
				require.ErrorIs(t, ValidateProposal(finished, terms), ErrRemainingKeyMismatch)
			}

			current, err := process.store.GetCurrent(authTestBeaconID)
			require.NoError(t, err)
			require.Equal(t, Complete, current.State)
			require.Equal(t, uint32(1), current.Epoch)
		})
	}
}

func TestPacketAcceptsProposalWithLeaver(t *testing.T) {
	leaderPair, leader := newKeyedParticipant(t, "leader")
	memberPair, member := newKeyedParticipant(t, "member")
	_, leaver := newKeyedParticipant(t, "leaver")
	_, fresh := newKeyedParticipant(t, "fresh")

	tests := []struct {
		name    string
		leaving *drand.Participant
		joining []*drand.Participant
	}{
		{name: "listed as in the last group", leaving: leaver},
		{name: "listed without key", leaving: &drand.Participant{Address: leaver.Address}},
		{name: "listed with a newer key", leaving: withAddressOf(fresh, leaver)},
		{name: "joining again with a new key", leaving: leaver, joining: []*drand.Participant{withAddressOf(fresh, leaver)}},
		{name: "joining again at a new address", leaving: leaver, joining: []*drand.Participant{withAddressOf(leaver, fresh)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			finished := NewCompleteDKGEntry(t, authTestBeaconID, Complete, leader, member, leaver)
			process := newPacketTestProcess(t, memberPair, finished)

			terms := NewValidProposal(authTestBeaconID, 2, leader, member)
			terms.Leaving = []*drand.Participant{test.leaving}
			terms.Joining = test.joining

			_, err := process.Packet(context.Background(), signedPacket(t, leaderPair, leader.Address, proposalPacket(terms), terms))
			require.NoError(t, err)

			current, err := process.store.GetCurrent(authTestBeaconID)
			require.NoError(t, err)
			require.Equal(t, Proposed, current.State)
			require.Len(t, current.Leaving, 1)
			require.Len(t, current.Joining, len(test.joining))
		})
	}
}

func TestPacketAbortMustBeSignedByLeader(t *testing.T) {
	leaderPair, leader := newKeyedParticipant(t, "leader")
	memberPair, member := newKeyedParticipant(t, "member")
	victimPair, victim := newKeyedParticipant(t, "victim")

	finished := NewCompleteDKGEntry(t, authTestBeaconID, Complete, leader, member, victim)
	process := newPacketTestProcess(t, victimPair, finished)
	currentState := func() *DBState {
		current, err := process.store.GetCurrent(authTestBeaconID)
		require.NoError(t, err)
		return current
	}

	terms := NewValidProposal(authTestBeaconID, 2, leader, member, victim)
	_, err := process.Packet(context.Background(), signedPacket(t, leaderPair, leader.Address, proposalPacket(terms), terms))
	require.NoError(t, err)
	require.Equal(t, Proposed, currentState().State)

	forged := signedPacket(t, memberPair, leader.Address, abortPacket(), termsFromState(currentState()))
	_, err = process.Packet(context.Background(), forged)
	require.ErrorContains(t, err, "invalid packet signature")
	require.Equal(t, Proposed, currentState().State)

	genuine := signedPacket(t, leaderPair, leader.Address, abortPacket(), termsFromState(currentState()))
	_, err = process.Packet(context.Background(), genuine)
	require.NoError(t, err)
	require.Equal(t, Aborted, currentState().State)
}

func TestVerifyMessageTakesRemainerKeysFromLastGroup(t *testing.T) {
	leaderPair, leader := newKeyedParticipant(t, "leader")
	memberPair, member := newKeyedParticipant(t, "member")
	lastGroup := NewCompleteDKGEntry(t, authTestBeaconID, Complete, leader, member).FinalGroup
	process := newPacketTestProcess(t, memberPair, nil)

	// the proposal lists the key of the member under the address of the leader
	terms := NewValidProposal(authTestBeaconID, 2, withAddressOf(member, leader))
	fromLeader := signedPacket(t, leaderPair, leader.Address, abortPacket(), terms)
	fromMember := signedPacket(t, memberPair, leader.Address, abortPacket(), terms)

	require.NoError(t, process.verifyMessage(fromLeader, terms, lastGroup))
	require.Error(t, process.verifyMessage(fromMember, terms, lastGroup))

	// without a finished group the proposal is the only source of keys
	require.Error(t, process.verifyMessage(fromLeader, terms, nil))
	require.NoError(t, process.verifyMessage(fromMember, terms, nil))

	// a remainer missing from the last group cannot be verified
	_, outsider := newKeyedParticipant(t, "outsider")
	terms.Remaining = append(terms.Remaining, withAddressOf(member, outsider))
	fromOutsider := signedPacket(t, memberPair, outsider.Address, abortPacket(), terms)
	require.Error(t, process.verifyMessage(fromOutsider, terms, lastGroup))
}

func TestPacketChecksProposalRosterBeforeSignatures(t *testing.T) {
	pair, me := newKeyedParticipant(t, "me")
	_, other := newKeyedParticipant(t, "other")
	// a well-formed signature that does not verify
	badSignature := make([]byte, len(me.Signature))

	repeated := make([]*drand.Participant, MaxParticipants)
	for i := range repeated {
		repeated[i] = me
	}

	tooMany := make([]*drand.Participant, MaxParticipants+1)
	for i := range tooMany {
		name := fmt.Sprintf("node%d:443", i)
		tooMany[i] = &drand.Participant{Address: name, Key: []byte(name)}
	}

	tests := []struct {
		name          string
		joining       []*drand.Participant
		expectedError error
	}{
		{name: "repeated joiners", joining: repeated, expectedError: ErrDuplicateParticipant},
		{name: "too many joiners", joining: tooMany, expectedError: ErrTooManyParticipants},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			process := newPacketTestProcess(t, pair, nil)
			terms := NewInitialProposal(authTestBeaconID, me)
			terms.Joining = test.joining
			packet := proposalPacket(terms)
			packet.Metadata = &drand.GossipMetadata{BeaconID: authTestBeaconID, Address: me.Address, Signature: badSignature}

			_, err := process.Packet(context.Background(), packet)
			require.ErrorIs(t, err, test.expectedError)
		})
	}

	t.Run("packet signature is verified before joiner signatures", func(t *testing.T) {
		process := newPacketTestProcess(t, pair, nil)
		invalidJoiner := &drand.Participant{Address: other.Address, Key: other.Key, Signature: badSignature}
		terms := NewInitialProposal(authTestBeaconID, me, invalidJoiner)
		packet := proposalPacket(terms)
		packet.Metadata = &drand.GossipMetadata{BeaconID: authTestBeaconID, Address: me.Address, Signature: badSignature}

		_, err := process.Packet(context.Background(), packet)
		require.ErrorContains(t, err, "invalid packet signature")
		require.NotErrorIs(t, err, key.ErrInvalidKeyScheme)

		// with a valid packet signature, the joiner signature is what gets rejected
		_, err = process.Packet(context.Background(), signedPacket(t, pair, me.Address, proposalPacket(terms), terms))
		require.ErrorIs(t, err, key.ErrInvalidKeyScheme)
	})
}

func TestReshareFromMigratedGroup(t *testing.T) {
	leaderPair, leader := newKeyedParticipant(t, "leader")
	followerPair, follower := newKeyedParticipant(t, "follower")

	// a v1 group holds signatures that are not valid in v2
	group := NewCompleteDKGEntry(t, authTestBeaconID, Complete, leader, follower).FinalGroup
	for _, node := range group.Nodes {
		node.Signature = []byte("v1 signature")
	}
	group.PublicKey = fakePublic()

	process := newPacketTestProcess(t, followerPair, nil)
	require.NoError(t, process.Migrate(authTestBeaconID, group, fakeShare()))
	migrated, err := process.store.GetFinished(authTestBeaconID)
	require.NoError(t, err)
	for _, p := range migrated.Joining {
		require.Nil(t, p.Signature)
	}

	terms := NewValidProposal(authTestBeaconID, 2, leader, follower)

	_, err = migrated.Proposing(leader, terms)
	require.NoError(t, err)

	_, err = process.Packet(context.Background(), signedPacket(t, leaderPair, leader.Address, proposalPacket(terms), terms))
	require.NoError(t, err)
	current, err := process.store.GetCurrent(authTestBeaconID)
	require.NoError(t, err)
	require.Equal(t, Proposed, current.State)
	require.Equal(t, uint32(2), current.Epoch)
}
