package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

type testSSHServer struct {
	address string
	close   func() error
}

func generateTestSSHSigner(t *testing.T) ssh.Signer {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("build test signer: %v", err)
	}

	return signer
}

func startTestSSHServer(t *testing.T, hostSigner ssh.Signer, clientPublicKey ssh.PublicKey) testSSHServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test SSH server: %v", err)
	}

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), clientPublicKey.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("rejected public key for %s", conn.User())
		},
	}
	config.AddHostKey(hostSigner)

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer close(done)

		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				t.Logf("test SSH accept error: %v", err)
				return
			}

			wg.Add(1)
			go func(conn net.Conn) {
				defer wg.Done()
				handleTestSSHConnection(t, conn, config)
			}(conn)
		}
	}()

	closeServer := func() error {
		err := listener.Close()
		<-done
		wg.Wait()
		return err
	}
	t.Cleanup(func() {
		if err := closeServer(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("close test SSH server: %v", err)
		}
	})

	return testSSHServer{
		address: listener.Addr().String(),
		close:   closeServer,
	}
}

func handleTestSSHConnection(t *testing.T, conn net.Conn, config *ssh.ServerConfig) {
	t.Helper()

	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		t.Logf("test SSH handshake error: %v", err)
		return
	}
	go ssh.DiscardRequests(requests)

	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			t.Logf("test SSH accept channel error: %v", err)
			continue
		}

		go handleTestSSHSession(t, channel, requests)
	}
}

func handleTestSSHSession(t *testing.T, channel ssh.Channel, requests <-chan *ssh.Request) {
	t.Helper()
	defer channel.Close()

	for request := range requests {
		switch request.Type {
		case "exec":
			if request.WantReply {
				_ = request.Reply(true, nil)
			}
			_, _ = io.WriteString(channel, "test ssh fixture\n")
			_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			return
		default:
			if request.WantReply {
				_ = request.Reply(false, nil)
			}
		}
	}
}

func testSSHClientConfig(hostPublicKey ssh.PublicKey, clientSigner ssh.Signer) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: "test-user",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(clientSigner),
		},
		HostKeyCallback: ssh.FixedHostKey(hostPublicKey),
	}
}

func TestSSHTransportFixtureAcceptsCommandSession(t *testing.T) {
	hostSigner := generateTestSSHSigner(t)
	clientSigner := generateTestSSHSigner(t)
	server := startTestSSHServer(t, hostSigner, clientSigner.PublicKey())

	client, err := ssh.Dial("tcp", server.address, testSSHClientConfig(hostSigner.PublicKey(), clientSigner))
	if err != nil {
		t.Fatalf("dial test SSH server: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("open test SSH session: %v", err)
	}
	defer session.Close()

	output, err := session.Output("fixture-check")
	if err != nil {
		t.Fatalf("run test SSH command: %v", err)
	}

	if string(output) != "test ssh fixture\n" {
		t.Fatalf("output = %q, want %q", string(output), "test ssh fixture\n")
	}
}
