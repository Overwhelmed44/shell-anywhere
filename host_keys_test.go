package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func testHostPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

func TestHostKeyMismatchConfirmation(t *testing.T) {
	app, host, sessionID, _ := testSFTPConnection(t)
	app.DisconnectSession(sessionID)
	path := filepath.Join(app.configDir, "known_hosts")
	address := net.JoinHostPort(host.Address, strconv.Itoa(host.Port))
	oldKey := testHostPublicKey(t)
	before := knownhosts.Line([]string{knownhosts.Normalize(address)}, oldKey) + "\n"
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	challenge, err := app.ReconnectSFTP(sessionID, "personal", host.ID, "", "", "")
	if err != nil || challenge.Status != "host_key_mismatch" || len(challenge.PreviousFingerprints) != 1 || challenge.PreviousFingerprints[0] != ssh.FingerprintSHA256(oldKey) {
		t.Fatalf("expected mismatch challenge: %+v, %v", challenge, err)
	}
	for _, confirmation := range []string{challenge.Fingerprint, "replace:wrong-fingerprint"} {
		result, err := app.ConnectHost(host.ID, "", "", confirmation)
		if err != nil || result.Status != "host_key_mismatch" {
			t.Fatalf("replacement without the exact mismatch approval must be refused: %+v, %v", result, err)
		}
	}
	if _, err := app.ConnectSFTP("personal", host.ID, "wrong-password", "", challenge.Confirmation); err == nil {
		t.Fatal("bad credentials should fail before saving the replacement key")
	}
	if current, err := os.ReadFile(path); err != nil || string(current) != before {
		t.Fatalf("unconfirmed or failed attempts changed the saved key: %v", err)
	}
	result, err := app.ReconnectSFTP(sessionID, "personal", host.ID, "", "", challenge.Confirmation)
	if err != nil || result.Status != "connected" || result.SessionID != sessionID {
		t.Fatalf("confirmed replacement must reconnect the existing session: %+v, %v", result, err)
	}
	app.DisconnectSession(sessionID)
	result, err = app.ConnectSFTP("personal", host.ID, "", "", "")
	if err != nil || result.Status != "connected" {
		t.Fatalf("replacement must be remembered: %+v, %v", result, err)
	}
	app.DisconnectSession(result.SessionID)
}

func TestReplaceHostKeyPreservesOtherHosts(t *testing.T) {
	oldKey, newKey := testHostPublicKey(t), testHostPublicKey(t)
	address := "server.example:22"
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
	for _, hosts := range []string{
		"server.example", "server.example,other.example", "*.example", knownhosts.HashHostname("server.example"),
	} {
		t.Run(hosts, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "known_hosts")
			before := "# Keep this comment\n" + knownhosts.Line([]string{hosts}, oldKey) + "\n" + knownhosts.Line([]string{"unrelated.test"}, oldKey) + "\n"
			if err := os.WriteFile(path, []byte(before), 0600); err != nil {
				t.Fatal(err)
			}
			if err := persistTrustedHostKey(path, address, remote, newKey, false); err == nil {
				t.Fatal("replacement requires explicit approval")
			}
			if err := persistTrustedHostKey(path, address, remote, newKey, true); err != nil {
				t.Fatal(err)
			}
			verify, err := knownhosts.New(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := verify(address, remote, newKey); err != nil {
				t.Fatalf("new key not trusted: %v", err)
			}
			if err := verify(address, remote, oldKey); err == nil {
				t.Fatal("old key must no longer be trusted for the replaced host")
			}
			if err := verify("unrelated.test:22", remote, oldKey); err != nil {
				t.Fatalf("unrelated host changed: %v", err)
			}
			if hosts == "server.example,other.example" || hosts == "*.example" {
				if err := verify("other.example:22", remote, oldKey); err != nil {
					t.Fatalf("shared entry lost another host: %v", err)
				}
			}
		})
	}
}
