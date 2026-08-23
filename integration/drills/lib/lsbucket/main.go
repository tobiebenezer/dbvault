// Command lsbucket lists object keys on a live S3 endpoint using the same
// aws-sdk-go-v2 dependency the production s3 adapter uses. Drill-harness
// plumbing only.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: lsbucket <endpoint> <bucket> <access-key> <secret-key>")
		os.Exit(2)
	}
	endpoint, bucket, access, secret := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
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
	var continuation *string
	for {
		out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			ContinuationToken: continuation,
		})
		if err != nil {
			fail(err)
		}
		for _, obj := range out.Contents {
			fmt.Println(aws.ToString(obj.Key))
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return
		}
		continuation = out.NextContinuationToken
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lsbucket:", err)
	os.Exit(1)
}
