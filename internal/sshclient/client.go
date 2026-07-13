package sshclient

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func Dial(h config.Host) (*ssh.Client, *sftp.Client, error) {
	auths := []ssh.AuthMethod{}

	// Try ssh-agent first
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			defer conn.Close()
			agentClient := agent.NewClient(conn)
			auths = append(auths, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	// Try identity files
	if h.IdentityFile != "" {
		auths = append(auths, publicKeyFile(h.IdentityFile))
	} else {
		for _, keyfile := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
			home, _ := os.UserHomeDir()
			keypath := filepath.Join(home, ".ssh", keyfile)
			auths = append(auths, publicKeyFile(keypath))
		}
	}

	config := &ssh.ClientConfig{
		User:            h.User,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := net.JoinHostPort(h.Hostname, fmt.Sprintf("%d", h.Port))

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, nil, err
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, nil, err
	}

	return client, sftpClient, nil
}

func publicKeyFile(file string) ssh.AuthMethod {
	return ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
		key, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}

		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, err
		}
		return []ssh.Signer{signer}, nil
	})
}
