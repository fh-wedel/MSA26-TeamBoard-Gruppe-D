import {
  DynamoDBClient,
  CreateTableCommand,
  DescribeTableCommand,
} from "@aws-sdk/client-dynamodb";
import { DynamoDBDocumentClient, QueryCommand } from "@aws-sdk/lib-dynamodb";

export interface ConnectionRecord {
  connectionId: string;
  boardId: number;
  ttl: number;
}

export interface ConnectionStore {
  byBoard(boardId: number): Promise<ConnectionRecord[]>;
}

export class DynamoConnectionStore implements ConnectionStore {
  private readonly doc: DynamoDBDocumentClient;

  constructor(
    client: DynamoDBClient,
    private readonly tableName: string,
  ) {
    this.doc = DynamoDBDocumentClient.from(client);
  }

  async byBoard(boardId: number): Promise<ConnectionRecord[]> {
    const out = await this.doc.send(
      new QueryCommand({
        TableName: this.tableName,
        IndexName: "boardId-index",
        KeyConditionExpression: "boardId = :b",
        ExpressionAttributeValues: { ":b": boardId },
      }),
    );
    return (out.Items ?? []) as ConnectionRecord[];
  }
}

export async function ensureTable(
  client: DynamoDBClient,
  tableName: string,
): Promise<void> {
  try {
    await client.send(new DescribeTableCommand({ TableName: tableName }));
    return;
  } catch (err) {
    const e = err as { name?: string };
    if (e.name !== "ResourceNotFoundException") throw err;
  }
  await client.send(
    new CreateTableCommand({
      TableName: tableName,
      AttributeDefinitions: [
        { AttributeName: "connectionId", AttributeType: "S" },
        { AttributeName: "boardId", AttributeType: "N" },
      ],
      KeySchema: [{ AttributeName: "connectionId", KeyType: "HASH" }],
      BillingMode: "PAY_PER_REQUEST",
      GlobalSecondaryIndexes: [
        {
          IndexName: "boardId-index",
          KeySchema: [{ AttributeName: "boardId", KeyType: "HASH" }],
          Projection: { ProjectionType: "ALL" },
        },
      ],
    }),
  );
}
