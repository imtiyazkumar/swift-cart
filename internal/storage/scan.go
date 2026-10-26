package storage

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

type scannedStore struct{ Store }

func withScan(s Store) Store { return scannedStore{s} }
func (s scannedStore) Put(ctx context.Context, key, contentType string, body io.ReadSeeker) error {
	address := os.Getenv("DOCUMENT_CLAMAV_ADDRESS")
	if address == "" {
		if os.Getenv("ENVIRONMENT") == "production" {
			return errors.New("document scanner is required")
		}
		return s.Store.Put(ctx, key, contentType, body)
	}
	connection, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return errors.New("document scanner unavailable")
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err = connection.Write([]byte("zINSTREAM\x00")); err != nil {
		return errors.New("document scanner unavailable")
	}
	buffer := make([]byte, 32768)
	size := make([]byte, 4)
	for {
		n, readErr := body.Read(buffer)
		if n > 0 {
			binary.BigEndian.PutUint32(size, uint32(n))
			if _, err = connection.Write(size); err != nil {
				return err
			}
			if _, err = connection.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if _, err = connection.Write([]byte{0, 0, 0, 0}); err != nil {
		return err
	}
	response := make([]byte, 1024)
	n, err := connection.Read(response)
	if err != nil {
		return errors.New("document scan failed")
	}
	if strings.TrimRight(string(response[:n]), "\x00\r\n") != "stream: OK" {
		return errors.New("document rejected by security scanner")
	}
	if _, err = body.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return s.Store.Put(ctx, key, contentType, body)
}
