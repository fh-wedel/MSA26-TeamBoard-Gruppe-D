import { DynamoDBClient } from "@aws-sdk/client-dynamodb";
import {
  ApiGatewayManagementApiClient,
  PostToConnectionCommand,
  GoneException,
} from "@aws-sdk/client-apigatewaymanagementapi";
import type { EventBridgeEvent } from "aws-lambda";
import { DynamoConnectionStore } from "./connections";

const logger = {
  info: (data: unknown, msg: string) =>
    console.log(JSON.stringify({ level: "info", msg, ...(data as object) })),
  error: (data: unknown, msg: string) =>
    console.error(JSON.stringify({ level: "error", msg, ...(data as object) })),
};

interface TicketMovedDetail {
  ticketId: number;
  boardId: number;
  from: string;
  to: string;
}

const region = process.env.AWS_REGION ?? "eu-central-1";
const tableName = process.env.CONNECTIONS_TABLE ?? "ws-connections";
const wsEndpoint = process.env.WS_CALLBACK_URL;

const dynamoClient = new DynamoDBClient({ region });
const store = new DynamoConnectionStore(dynamoClient, tableName);
const apiGwClient = wsEndpoint
  ? new ApiGatewayManagementApiClient({ region, endpoint: wsEndpoint })
  : null;

export async function handler(
  event: EventBridgeEvent<string, TicketMovedDetail>,
): Promise<{ delivered: number; gone: number }> {
  if (!apiGwClient) {
    throw new Error("WS_CALLBACK_URL not configured");
  }
  const { boardId } = event.detail;
  const connections = await store.byBoard(boardId);
  logger.info(
    { boardId, count: connections.length, detailType: event["detail-type"] },
    "broadcasting event",
  );

  const payload = Buffer.from(
    JSON.stringify({ type: event["detail-type"], data: event.detail }),
  );

  let delivered = 0;
  let gone = 0;
  await Promise.all(
    connections.map(async (c) => {
      try {
        await apiGwClient.send(
          new PostToConnectionCommand({
            ConnectionId: c.connectionId,
            Data: payload,
          }),
        );
        delivered++;
      } catch (err) {
        if (err instanceof GoneException) {
          gone++;
          return;
        }
        logger.error(
          { err, connectionId: c.connectionId },
          "failed to post to connection",
        );
      }
    }),
  );
  return { delivered, gone };
}
