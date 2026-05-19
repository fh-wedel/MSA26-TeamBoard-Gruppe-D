import json
import os
from urllib.parse import unquote_plus

import boto3

s3 = boto3.client("s3")
dynamodb = boto3.resource("dynamodb")

AUDIT_TABLE = os.environ.get("AUDIT_TABLE", "")


def handler(event, context):
    processed = []
    table = dynamodb.Table(AUDIT_TABLE) if AUDIT_TABLE else None

    for record in event.get("Records", []):
        bucket = record["s3"]["bucket"]["name"]
        key = unquote_plus(record["s3"]["object"]["key"])
        head = s3.head_object(Bucket=bucket, Key=key)
        item = {
            "bucket": bucket,
            "key": key,
            "size": head.get("ContentLength", 0),
            "content_type": head.get("ContentType", "application/octet-stream"),
            "metadata": head.get("Metadata", {}),
        }
        processed.append(item)
        if table is not None:
            table.put_item(
                Item={
                    "pk": "DOCUMENT_PROCESSOR",
                    "sk": key,
                    "event_type": "document_uploaded",
                    **item,
                }
            )

    return {
        "statusCode": 200,
        "body": json.dumps({"processed": processed}),
    }
