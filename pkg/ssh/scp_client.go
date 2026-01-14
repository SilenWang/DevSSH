package ssh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SCPClient struct {
	client *Client
}

func NewSCPClient(client *Client) *SCPClient {
	return &SCPClient{
		client: client,
	}
}

func (s *SCPClient) Upload(localPath, remotePath string) error {
	if !s.client.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	if fileInfo.IsDir() {
		return fmt.Errorf("local path is a directory, only file upload is supported")
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	remoteDir := filepath.Dir(remotePath)
	if remoteDir != "." && remoteDir != "/" {
		mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteDir)
		if _, err := s.client.RunCommand(mkdirCmd); err != nil {
			return fmt.Errorf("failed to create remote directory: %w", err)
		}
	}

	return s.uploadViaSSH(file, remotePath, fileInfo.Size(), fileInfo.Mode())
}

func (s *SCPClient) uploadViaSSH(file *os.File, remotePath string, size int64, mode os.FileMode) error {
	session, err := s.client.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := session.Start(fmt.Sprintf("scp -t %s", remotePath)); err != nil {
		return fmt.Errorf("failed to start SCP command: %w", err)
	}

	errors := make(chan error, 2)
	go func() {
		_, err := io.Copy(io.Discard, stdout)
		errors <- err
	}()

	go func() {
		defer stdin.Close()

		fmt.Fprintf(stdin, "C%04o %d %s\n", mode&0777, size, filepath.Base(remotePath))

		buf := make([]byte, 32*1024)
		_, err := io.CopyBuffer(stdin, file, buf)
		if err != nil {
			errors <- err
			return
		}

		fmt.Fprint(stdin, "\x00")
		errors <- nil
	}()

	err1 := <-errors
	err2 := <-errors

	if err := session.Wait(); err != nil {
		return fmt.Errorf("SCP command failed: %w", err)
	}

	if err1 != nil {
		return fmt.Errorf("stdout error: %w", err1)
	}
	if err2 != nil {
		return fmt.Errorf("stdin error: %w", err2)
	}

	return nil
}

func (s *SCPClient) UploadWithReader(reader io.Reader, remotePath string, size int64) error {
	if !s.client.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	tempFile, err := os.CreateTemp("", "devssh-scp-upload-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, reader); err != nil {
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	fileInfo, err := os.Stat(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to stat temporary file: %w", err)
	}

	file, err := os.Open(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to open temporary file: %w", err)
	}
	defer file.Close()

	return s.uploadViaSSH(file, remotePath, fileInfo.Size(), fileInfo.Mode())
}

func (s *SCPClient) Download(remotePath, localPath string) error {
	if !s.client.IsConnected() {
		return fmt.Errorf("SSH client not connected")
	}

	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
	}

	session, err := s.client.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := session.Start(fmt.Sprintf("scp -f %s", remotePath)); err != nil {
		return fmt.Errorf("failed to start SCP command: %w", err)
	}

	fmt.Fprint(stdin, "\x00")

	response := make([]byte, 1)
	if _, err := stdout.Read(response); err != nil {
		return fmt.Errorf("failed to read SCP response: %w", err)
	}

	if response[0] != 0 {
		return fmt.Errorf("SCP error response: %d", response[0])
	}

	fmt.Fprint(stdin, "\x00")

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(file, stdout, buf)
	if err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}

	fmt.Fprint(stdin, "\x00")

	if err := session.Wait(); err != nil {
		return fmt.Errorf("SCP command failed: %w", err)
	}

	return nil
}

func (s *SCPClient) CheckRemoteFileExists(remotePath string) (bool, error) {
	if !s.client.IsConnected() {
		return false, fmt.Errorf("SSH client not connected")
	}

	checkCmd := fmt.Sprintf("test -f %s && echo exists", remotePath)
	output, err := s.client.RunCommand(checkCmd)
	if err != nil {
		return false, nil
	}

	return strings.Contains(output, "exists"), nil
}

func (s *SCPClient) GetRemoteFileSize(remotePath string) (int64, error) {
	if !s.client.IsConnected() {
		return 0, fmt.Errorf("SSH client not connected")
	}

	sizeCmd := fmt.Sprintf("stat -c %%s %s 2>/dev/null || wc -c < %s 2>/dev/null", remotePath, remotePath)
	output, err := s.client.RunCommand(sizeCmd)
	if err != nil {
		return 0, fmt.Errorf("failed to get remote file size: %w", err)
	}

	var size int64
	if _, err := fmt.Sscanf(strings.TrimSpace(output), "%d", &size); err != nil {
		return 0, fmt.Errorf("failed to parse file size: %w", err)
	}

	return size, nil
}
