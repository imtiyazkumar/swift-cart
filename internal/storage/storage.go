package storage

import (
    "context"
    "errors"
    "io"
    "os"
    "path/filepath"

    "github.com/aws/aws-sdk-go-v2/aws"
    awsconfig "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/google/uuid"
)

type Store interface {
    Put(context.Context, string, string, io.ReadSeeker) error
    Open(context.Context, string) (io.ReadCloser, error)
    Delete(context.Context, string) error
}

func Configured(ctx context.Context, backend string) (Store, error) {
    switch backend {
    case "local":
        directory := os.Getenv("DOCUMENT_STORAGE_DIRECTORY")
        if directory == "" {
            return nil, errors.New("private document directory is not configured")
        }
        return Local{Directory: directory}, nil
    case "s3":
        bucket := os.Getenv("DOCUMENT_S3_BUCKET")
        if bucket == "" {
            return nil, errors.New("private document S3 bucket is not configured")
        }
        cfg, err := awsconfig.LoadDefaultConfig(ctx)
        if err != nil {
            return nil, errors.New("S3 configuration is unavailable")
        }
        client := s3.NewFromConfig(cfg, func(options *s3.Options) {
            if endpoint := os.Getenv("DOCUMENT_S3_ENDPOINT"); endpoint != "" {
                options.BaseEndpoint = aws.String(endpoint)
                options.UsePathStyle = true
            }
        })
        return S3{Client: client, Bucket: bucket}, nil
    default:
        return nil, errors.New("document storage backend must be local or s3")
    }
}

type Local struct{ Directory string }

func (s Local) path(key string) (string, error) {
    id, err := uuid.Parse(key)
    if err != nil {
        return "", errors.New("invalid storage key")
    }
    return filepath.Join(s.Directory, id.String()), nil
}
func (s Local) Put(ctx context.Context, key, contentType string, body io.ReadSeeker) error {
    path, err := s.path(key)
    if err != nil {
        return err
    }
    if err = os.MkdirAll(s.Directory, 0700); err != nil {
        return err
    }
    file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
    if err != nil {
        return err
    }
    _, copyErr := io.Copy(file, body)
    closeErr := file.Close()
    if copyErr != nil || closeErr != nil {
        _ = os.Remove(path)
        return errors.New("document write failed")
    }
    return nil
}
func (s Local) Open(ctx context.Context, key string) (io.ReadCloser, error) {
    path, err := s.path(key)
    if err != nil {
        return nil, err
    }
    return os.Open(path)
}
func (s Local) Delete(ctx context.Context, key string) error {
    path, err := s.path(key)
    if err != nil {
        return err
    }
    return os.Remove(path)
}

type S3 struct {
    Client *s3.Client
    Bucket string
}

func (s S3) Put(ctx context.Context, key, contentType string, body io.ReadSeeker) error {
    if _, err := uuid.Parse(key); err != nil {
        return errors.New("invalid storage key")
    }
    _, err := s.Client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String("private-documents/" + key), Body: body, ContentType: aws.String(contentType)})
    return err
}
func (s S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
    if _, err := uuid.Parse(key); err != nil {
        return nil, errors.New("invalid storage key")
    }
    result, err := s.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String("private-documents/" + key)})
    if err != nil {
        return nil, err
    }
    return result.Body, nil
}
func (s S3) Delete(ctx context.Context, key string) error {
    if _, err := uuid.Parse(key); err != nil {
        return errors.New("invalid storage key")
    }
    _, err := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Bucket), Key: aws.String("private-documents/" + key)})
    return err
}
