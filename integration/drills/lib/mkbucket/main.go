// Command mkbucket creates an S3 bucket on a live endpoint using the same
// aws-sdk-go-v2 dependency the production s3 adapter uses, so the MinIO drill
// does not need the minio/mc image. Drill-harness plumbing only.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func main() {
	if len(os.Args) != 6 {
		fmt.Fprintln(os.Stderr, "usage: mkbucket <endpoint> <region> <bucket> <access-key> <secret-key>")
		os.Exit(2)
	}
	endpoint, region, bucket, access, secret := os.Args[1], os.Args[2], os.Args[3], os.Args[4], os.Args[5]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: access, SecretAccessKey: secret}, nil
		})),
	)
	if err != nil {
		fail(err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		var owned *s3types.BucketAlreadyOwnedByYou
		if !errors.As(err, &owned) {
			fail(err)
		}
	}
	fmt.Printf("[drill] bucket %s ready at %s\n", bucket, endpoint)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mkbucket:", err)
	os.Exit(1)
}
