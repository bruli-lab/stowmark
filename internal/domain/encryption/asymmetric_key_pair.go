package encryption

import "fmt"

type AsymmetricKeyPair struct {
	publicKey  string
	privateKey string
}

func (a AsymmetricKeyPair) PublicKey() string {
	return a.publicKey
}

func (a AsymmetricKeyPair) PrivateKey() string {
	return a.privateKey
}

func NewAsymmetricKeyPair(folder string) (*AsymmetricKeyPair, error) {
	id, err := generateKeyID()
	if err != nil {
		return nil, err
	}
	privateKey := fmt.Sprintf("%s/%s-private.pem", folder, id)
	publicKey := fmt.Sprintf("%s/%s-public.pem", folder, id)
	return &AsymmetricKeyPair{
		publicKey:  publicKey,
		privateKey: privateKey,
	}, nil
}
