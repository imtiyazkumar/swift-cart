package storage

import (
    "bytes"
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/google/uuid"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestLocalRejectsTraversalAndPreservesDocument(t *testing.T) {
    ctx := context.Background()
    store := Local{Directory: t.TempDir()}
    id := uuid.NewString()
    if err := store.Put(ctx, "../secret", "application/pdf", bytes.NewReader([]byte("test"))); err == nil {
        t.Fatal("path traversal accepted")
    }
    if err := store.Put(ctx, id, "application/pdf", bytes.NewReader([]byte("test"))); err != nil {
        t.Fatal(err)
    }
    if err := store.Put(ctx, id, "application/pdf", bytes.NewReader([]byte("overwrite"))); err == nil {
        t.Fatal("existing document overwritten")
    }
    file, err := store.Open(ctx, id)
    if err != nil {
        t.Fatal(err)
    }
    data, _ := io.ReadAll(file)
    _ = file.Close()
    if string(data) != "test" {
        t.Fatal("document content changed")
    }
}

func TestS3UsesPrivateAuthenticatedObjectRequests(t *testing.T) {
    var data []byte
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
            t.Error("unsigned storage request")
        }
        if r.Header.Get("X-Amz-Acl") == "public-read" {
            t.Error("private document uploaded publicly")
        }
        switch r.Method {
        case "PUT":
            data, _ = io.ReadAll(r.Body)
            w.WriteHeader(200)
        case "GET":
            w.Header().Set("Content-Type", "application/pdf")
            _, _ = w.Write(data)
        case "DELETE":
            w.WriteHeader(204)
        }
    }))
    defer server.Close()
    client := s3.New(s3.Options{Region: "us-east-1", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
        return aws.Credentials{AccessKeyID: "local-test", SecretAccessKey: "local-test-secret"}, nil
    })})
    store := S3{Client: client, Bucket: "private-test"}
    ctx := context.Background()
    key := uuid.NewString()
    if err := store.Put(ctx, key, "application/pdf", bytes.NewReader([]byte("test-document"))); err != nil {
        t.Fatal(err)
    }
    file, err := store.Open(ctx, key)
    if err != nil {
        t.Fatal(err)
    }
    received, _ := io.ReadAll(file)
    _ = file.Close()
    if string(received) != "test-document" {
        t.Fatal("S3 round trip failed")
    }
    if err = store.Delete(ctx, key); err != nil {
        t.Fatal(err)
    }
}
