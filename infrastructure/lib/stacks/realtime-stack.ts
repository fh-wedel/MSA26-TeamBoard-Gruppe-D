import { Stack, StackProps, Duration } from "aws-cdk-lib";
import * as apigwv2 from "aws-cdk-lib/aws-apigatewayv2";
import * as integrations from "aws-cdk-lib/aws-apigatewayv2-integrations";
import * as dynamodb from "aws-cdk-lib/aws-dynamodb";
import * as events from "aws-cdk-lib/aws-events";
import * as targets from "aws-cdk-lib/aws-events-targets";
import * as iam from "aws-cdk-lib/aws-iam";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as logs from "aws-cdk-lib/aws-logs";
import * as nodejs from "aws-cdk-lib/aws-lambda-nodejs";
import { Construct } from "constructs";
import * as path from "path";

export interface RealtimeStackProps extends StackProps {
  eventBus: events.EventBus;
  connectionsTable: dynamodb.Table;
}

export class RealtimeStack extends Stack {
  public readonly webSocketApi: apigwv2.WebSocketApi;
  public readonly broadcaster: lambda.Function;

  constructor(scope: Construct, id: string, props: RealtimeStackProps) {
    super(scope, id, props);

    // Connection manager Lambda used by the WebSocket API ($connect, $disconnect)
    const connectionManager = new nodejs.NodejsFunction(this, "ConnectionManager", {
      entry: path.join(__dirname, "../../../lambdas/ws-broadcaster/src/handler.ts"),
      handler: "handler",
      runtime: lambda.Runtime.NODEJS_20_X,
      timeout: Duration.seconds(10),
      memorySize: 256,
      environment: {
        CONNECTIONS_TABLE: props.connectionsTable.tableName,
      },
      logRetention: logs.RetentionDays.ONE_WEEK,
      bundling: { externalModules: ["@aws-sdk/*"], minify: true },
    });
    props.connectionsTable.grantReadWriteData(connectionManager);

    this.webSocketApi = new apigwv2.WebSocketApi(this, "WebSocketApi", {
      apiName: "msa2-realtime",
      connectRouteOptions: {
        integration: new integrations.WebSocketLambdaIntegration(
          "ConnectIntegration",
          connectionManager,
        ),
      },
      disconnectRouteOptions: {
        integration: new integrations.WebSocketLambdaIntegration(
          "DisconnectIntegration",
          connectionManager,
        ),
      },
    });
    const wsStage = new apigwv2.WebSocketStage(this, "WebSocketStage", {
      webSocketApi: this.webSocketApi,
      stageName: "prod",
      autoDeploy: true,
    });

    // Broadcaster Lambda — triggered by EventBridge, pushes to API Gateway @connections
    this.broadcaster = new nodejs.NodejsFunction(this, "Broadcaster", {
      entry: path.join(__dirname, "../../../lambdas/ws-broadcaster/src/handler.ts"),
      handler: "handler",
      runtime: lambda.Runtime.NODEJS_20_X,
      timeout: Duration.seconds(15),
      memorySize: 512,
      environment: {
        CONNECTIONS_TABLE: props.connectionsTable.tableName,
        WS_CALLBACK_URL: `https://${this.webSocketApi.apiId}.execute-api.${this.region}.amazonaws.com/${wsStage.stageName}`,
      },
      logRetention: logs.RetentionDays.ONE_WEEK,
      bundling: { externalModules: ["@aws-sdk/*"], minify: true },
    });
    props.connectionsTable.grantReadData(this.broadcaster);
    this.broadcaster.addToRolePolicy(
      new iam.PolicyStatement({
        actions: ["execute-api:ManageConnections"],
        resources: [
          `arn:aws:execute-api:${this.region}:${this.account}:${this.webSocketApi.apiId}/${wsStage.stageName}/*/@connections/*`,
        ],
      }),
    );

    new events.Rule(this, "TicketMovedRule", {
      eventBus: props.eventBus,
      eventPattern: {
        source: events.Match.prefix("plugin."),
        detailType: ["ticket.moved", "ticket.assigned", "ticket.created"],
      },
      targets: [new targets.LambdaFunction(this.broadcaster)],
    });
  }
}
