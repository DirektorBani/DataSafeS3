import { S3Client, ListBucketsCommand } from "@aws-sdk/client-s3";

const client = new S3Client({
  endpoint: process.env.DATASAFE_ENDPOINT ?? "http://127.0.0.1:9000",
  region: process.env.DATASAFE_REGION ?? "us-east-1",
  credentials: {
    accessKeyId: process.env.DATASAFE_ACCESS_KEY ?? "datasafe",
    secretAccessKey: process.env.DATASAFE_SECRET_KEY ?? "datasafesecret",
  },
  forcePathStyle: true,
});

const out = await client.send(new ListBucketsCommand({}));
console.log(`Buckets: ${out.Buckets?.length ?? 0}`);
