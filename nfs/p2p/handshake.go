package p2p

type HandshakeFunc func(peer Peer) error

func NOOPHandshaker(peer Peer) error {
	//some peer to peer handshake stuff here, certificate verifications etc
	return nil
}
