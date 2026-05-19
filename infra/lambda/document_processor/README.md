# Document Processor Lambda

Example Lambda target for S3 `ObjectCreated` events from the TeamBoard document bucket.

Best-fit use cases:

- Store upload audit metadata in DynamoDB.
- Trigger virus scanning.
- Extract text from uploaded files.
- Create thumbnails/previews.
- Generate Bedrock embeddings or summaries asynchronously.

This Lambda is intentionally not required for the local MVP. It is a cloud extension point for the S3 document storage path.
