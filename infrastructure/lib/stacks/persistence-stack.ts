import { Duration, RemovalPolicy, Stack, StackProps } from "aws-cdk-lib";
import * as dynamodb from "aws-cdk-lib/aws-dynamodb";
import * as s3 from "aws-cdk-lib/aws-s3";
import { Construct } from "constructs";

export type PersistenceStackProps = StackProps;

/**
 * Serverless persistence layer for the PoC.
 *
 * Architectural choice: we use DynamoDB for both the plugin registry (TTL
 * attribute lets entries expire automatically when heartbeats stop) and the
 * Kanban tickets (composite key BOARD#<id> / TICKET#<uuid> matches our access
 * patterns). No Aurora, no ElastiCache — those add cost and ~15 minutes of
 * provisioning to every fresh deploy without earning anything for our access
 * patterns. Locally we still use Postgres + Redis via docker-compose; the
 * services pick a backend via the STORAGE / REGISTRY_BACKEND / EVENT_BUS env
 * vars.
 */
export class PersistenceStack extends Stack {
  public readonly ticketsTable: dynamodb.Table;
  public readonly pluginRegistryTable: dynamodb.Table;
  public readonly connectionsTable: dynamodb.Table;
  public readonly assetsBucket: s3.Bucket;

  constructor(scope: Construct, id: string, props: PersistenceStackProps = {}) {
    super(scope, id, props);

    this.ticketsTable = new dynamodb.Table(this, "TicketsTable", {
      tableName: "msa2-tickets",
      partitionKey: { name: "pk", type: dynamodb.AttributeType.STRING },
      sortKey: { name: "sk", type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      removalPolicy: RemovalPolicy.DESTROY,
    });

    this.pluginRegistryTable = new dynamodb.Table(this, "PluginRegistryTable", {
      tableName: "msa2-plugin-registry",
      partitionKey: { name: "pk", type: dynamodb.AttributeType.STRING },
      sortKey: { name: "pluginId", type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      timeToLiveAttribute: "ttl",
      removalPolicy: RemovalPolicy.DESTROY,
    });

    this.connectionsTable = new dynamodb.Table(this, "ConnectionsTable", {
      tableName: "ws-connections",
      partitionKey: { name: "connectionId", type: dynamodb.AttributeType.STRING },
      billingMode: dynamodb.BillingMode.PAY_PER_REQUEST,
      timeToLiveAttribute: "ttl",
      removalPolicy: RemovalPolicy.DESTROY,
    });
    this.connectionsTable.addGlobalSecondaryIndex({
      indexName: "boardId-index",
      partitionKey: { name: "boardId", type: dynamodb.AttributeType.NUMBER },
      projectionType: dynamodb.ProjectionType.ALL,
    });

    this.assetsBucket = new s3.Bucket(this, "AssetsBucket", {
      encryption: s3.BucketEncryption.S3_MANAGED,
      blockPublicAccess: s3.BlockPublicAccess.BLOCK_ALL,
      versioned: false,
      lifecycleRules: [{ abortIncompleteMultipartUploadAfter: Duration.days(7) }],
      removalPolicy: RemovalPolicy.DESTROY,
      autoDeleteObjects: true,
    });
  }
}
