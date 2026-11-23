# AWS S3 Setup & Operations Guide

## 1. AWS IAM Role Requirements (EC2)
- Attach the EC2 IAM Role **`research`** to the Ubuntu EC2 instance.
- **Do not** configure static `AWS_ACCESS_KEY_ID` or `AWS_SECRET_ACCESS_KEY` on EC2; the AWS SDK for Go v2 automatically uses the EC2 Instance Metadata Service (IMDSv2) credential chain.
- Keep **Block Public Access** enabled on the `researchimage` bucket.

### Required S3 Bucket Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::researchimage",
        "arn:aws:s3:::researchimage/*"
      ]
    }
  ]
}
```

## 2. Environment Configuration
Set the following in `.env` on EC2:
```env
STORAGE_DRIVER=s3
STORAGE_LOCAL_PATH=./storage
MAX_UPLOAD_SIZE_MB=25
S3_REGION=ap-southeast-2
S3_BUCKET=researchimage
S3_ENDPOINT=https://s3.ap-southeast-2.amazonaws.com
S3_ACCESS_KEY=
S3_SECRET_KEY=
```

To switch back to local development storage, set `STORAGE_DRIVER=local`.

## 3. Running Locally vs. on EC2
- **Local Development**: Set `STORAGE_DRIVER=local` in `.env` and run `go run main.go`.
- **EC2 Production**: Ensure `STORAGE_DRIVER=s3`, `S3_REGION=ap-southeast-2`, `S3_BUCKET=researchimage`, and run the compiled binary.

## 4. Testing Uploads & Downloads
- Upload a paper or logo via the Student Portal; verify the database stores an object key like `papers/<uuid>.pdf` or `logos/<uuid>.png`.
- Download the paper, certificate, or payment bill; the backend streams the private S3 object via `c.DataFromReader()` without exposing S3 URLs.

## 5. Migrating Existing Local Files
Call `cmd.MigrateLocalFilesToS3()` (with `STORAGE_DRIVER=s3` configured) to scan `PaperFile`, `Certificate`, and `Payment` records, upload missing files to S3 without overwriting existing objects, and update database paths only after upload succeeds. Original local files are preserved.

## 6. Troubleshooting
- **`STORAGE_DRIVER=s3 requires S3_REGION and S3_BUCKET`**: Verify `.env` has both variables set.
- **`AccessDenied`**: Confirm the EC2 instance has the `research` IAM role attached and the policy allows `PutObject`, `GetObject`, `DeleteObject`, and `ListBucket` on `researchimage`.
