package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"

	"golang.org/x/crypto/argon2"
)

type vaultHost struct {
	ID               string  `json:"-"`
	Name             string  `json:"name"`
	Address          string  `json:"address"`
	Port             int     `json:"port"`
	Username         string  `json:"username,omitempty"`
	AuthMethod       string  `json:"authMethod,omitempty"`
	Group            string  `json:"groupId,omitempty"`
	IdentityID       string  `json:"identityId,omitempty"`
	UseGroupIdentity bool    `json:"useGroupIdentity,omitempty"`
	Favorite         bool    `json:"favorite"`
	Password         *string `json:"password,omitempty"`
	PrivateKey       []byte  `json:"privateKey,omitempty"`
	Passphrase       *string `json:"passphrase,omitempty"`
}

type vaultData struct {
	Version    int             `json:"version"`
	Workspaces vaultWorkspaces `json:"workspaces"`
}

type vaultEnvelope struct {
	Version   int    `json:"version"`
	Protected bool   `json:"protected"`
	Salt      []byte `json:"salt"`
	Encrypted []byte `json:"encrypted"`
}

type vaultKey struct {
	Salt []byte `json:"salt"`
	Key  []byte `json:"key"`
}

func makeVaultKey(password string, salt []byte) vaultKey {
	return vaultKey{
		Salt: salt,
		Key:  argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32),
	}
}

func encodeVault(data []byte, key vaultKey) ([]byte, error) {
	var contents vaultData
	if json.Unmarshal(data, &contents) != nil || contents.Version != 1 {
		return nil, errors.New("unsupported cloud vault format")
	}
	if len(key.Key) != 32 {
		return nil, errors.New("a vault password is required to sync SSH credentials")
	}
	if err := validateVaultData(contents); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}

	envelope := vaultEnvelope{
		Version:   1,
		Protected: true,
		Salt:      key.Salt,
		Encrypted: gcm.Seal(nil, nil, data, []byte("shell-anywhere-vault:1")),
	}

	return json.Marshal(envelope)
}

func decodeVault(blob []byte, password string, saved vaultKey) ([]byte, vaultKey, bool, error) {
	var envelope vaultEnvelope

	if json.Unmarshal(blob, &envelope) != nil || envelope.Version != 1 {
		return nil, vaultKey{}, false, errors.New("unsupported cloud vault format")
	}

	if !envelope.Protected {
		return nil, vaultKey{}, false, errors.New("SSH credentials require an encrypted vault")
	}

	if len(envelope.Salt) != 16 || len(envelope.Encrypted) < 28 {
		return nil, vaultKey{}, true, errors.New("invalid encrypted cloud vault")
	}

	key := saved

	if password != "" {
		key = makeVaultKey(password, envelope.Salt)
	}

	if len(key.Key) != 32 || string(key.Salt) != string(envelope.Salt) {
		return nil, vaultKey{}, true, errors.New("enter your vault password to unlock cloud hosts")
	}

	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, vaultKey{}, true, err
	}

	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, vaultKey{}, true, err
	}

	data, err := gcm.Open(nil, nil, envelope.Encrypted, []byte("shell-anywhere-vault:1"))
	if err != nil {
		return nil, vaultKey{}, true, errors.New("the vault password is incorrect or the cloud vault is damaged")
	}

	return data, key, true, nil
}

func newVaultKey(password string) vaultKey {
	return makeVaultKey(password, randBytes(16))
}

func randBytes(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)

	return data
}
