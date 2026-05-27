import { Duration, RemovalPolicy, Stack, StackProps } from "aws-cdk-lib";
import * as dynamodb from "aws-cdk-lib/aws-dynamodb";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as elasticache from "aws-cdk-lib/aws-elasticache";
import * as rds from "aws-cdk-lib/aws-rds";
import * as s3 from "aws-cdk-lib/aws-s3";
import * as secretsmanager from "aws-cdk-lib/aws-secretsmanager";
import { Construct } from "constructs";

export interface PersistenceStackProps extends StackProps {
  vpc: ec2.IVpc;
}

export class PersistenceStack extends Stack {
  public readonly aurora: rds.DatabaseCluster;
  public readonly redis: elasticache.CfnReplicationGroup;
  public readonly redisSecurityGroup: ec2.SecurityGroup;
  public readonly connectionsTable: dynamodb.Table;
  public readonly assetsBucket: s3.Bucket;
  public readonly dbSecret: secretsmanager.ISecret;

  constructor(scope: Construct, id: string, props: PersistenceStackProps) {
    super(scope, id, props);

    // Aurora Postgres Serverless v2
    this.aurora = new rds.DatabaseCluster(this, "Aurora", {
      engine: rds.DatabaseClusterEngine.auroraPostgres({
        version: rds.AuroraPostgresEngineVersion.VER_16_8,
      }),
      vpc: props.vpc,
      vpcSubnets: { subnetType: ec2.SubnetType.PRIVATE_ISOLATED },
      credentials: rds.Credentials.fromGeneratedSecret("postgres"),
      defaultDatabaseName: "poc",
      writer: rds.ClusterInstance.serverlessV2("writer"),
      serverlessV2MinCapacity: 0.5,
      serverlessV2MaxCapacity: 2,
      removalPolicy: RemovalPolicy.DESTROY,
    });
    this.dbSecret = this.aurora.secret!;

    // ElastiCache Redis
    const redisSubnetGroup = new elasticache.CfnSubnetGroup(this, "RedisSubnetGroup", {
      description: "Subnets for ElastiCache Redis",
      subnetIds: props.vpc.selectSubnets({
        subnetType: ec2.SubnetType.PRIVATE_ISOLATED,
      }).subnetIds,
    });
    this.redisSecurityGroup = new ec2.SecurityGroup(this, "RedisSecurityGroup", {
      vpc: props.vpc,
      description: "Security group for ElastiCache Redis",
      allowAllOutbound: true,
    });

    this.redis = new elasticache.CfnReplicationGroup(this, "Redis", {
      replicationGroupDescription: "Plugin registry + pub/sub cache",
      engine: "redis",
      cacheNodeType: "cache.t4g.micro",
      numNodeGroups: 1,
      replicasPerNodeGroup: 0,
      automaticFailoverEnabled: false,
      cacheSubnetGroupName: redisSubnetGroup.ref,
      securityGroupIds: [this.redisSecurityGroup.securityGroupId],
      transitEncryptionEnabled: false,
      atRestEncryptionEnabled: true,
    });
    this.redis.addDependency(redisSubnetGroup);

    // DynamoDB connection table
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

    // S3 bucket for plugin assets / uploads
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
