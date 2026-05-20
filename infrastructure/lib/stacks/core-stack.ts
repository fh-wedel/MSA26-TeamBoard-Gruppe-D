import { Duration, Stack, StackProps } from "aws-cdk-lib";
import * as apigw from "aws-cdk-lib/aws-apigateway";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as ecs from "aws-cdk-lib/aws-ecs";
import * as ecs_patterns from "aws-cdk-lib/aws-ecs-patterns";
import * as events from "aws-cdk-lib/aws-events";
import * as logs from "aws-cdk-lib/aws-logs";
import * as rds from "aws-cdk-lib/aws-rds";
import * as elasticache from "aws-cdk-lib/aws-elasticache";
import * as secretsmanager from "aws-cdk-lib/aws-secretsmanager";
import { Construct } from "constructs";

export interface CoreStackProps extends StackProps {
  vpc: ec2.IVpc;
  aurora: rds.DatabaseCluster;
  redis: elasticache.CfnReplicationGroup;
  redisSecurityGroup: ec2.SecurityGroup;
  dbSecret: secretsmanager.ISecret;
  imageTag: string;
  ecrRepositoryName?: string;
}

export class CoreStack extends Stack {
  public readonly cluster: ecs.Cluster;
  public readonly eventBus: events.EventBus;
  public readonly coreService: ecs_patterns.ApplicationLoadBalancedFargateService;
  public readonly restApi: apigw.RestApi;

  constructor(scope: Construct, id: string, props: CoreStackProps) {
    super(scope, id, props);

    this.cluster = new ecs.Cluster(this, "Cluster", {
      vpc: props.vpc,
      containerInsights: true,
    });

    this.eventBus = new events.EventBus(this, "EventBus", {
      eventBusName: "msa2-bus",
    });

    const image = props.ecrRepositoryName
      ? ecs.ContainerImage.fromRegistry(
          `${this.account}.dkr.ecr.${this.region}.amazonaws.com/${props.ecrRepositoryName}:${props.imageTag}`,
        )
      : ecs.ContainerImage.fromAsset("../services/core");

    const redisEndpoint = props.redis.attrPrimaryEndPointAddress;
    const redisPort = props.redis.attrPrimaryEndPointPort;

    this.coreService = new ecs_patterns.ApplicationLoadBalancedFargateService(this, "CoreService", {
      cluster: this.cluster,
      cpu: 512,
      memoryLimitMiB: 1024,
      desiredCount: 1,
      publicLoadBalancer: true,
      taskImageOptions: {
        image,
        containerPort: 3000,
        environment: {
          CORE_PORT: "3000",
          NODE_ENV: "production",
          EVENT_BUS: "eventbridge",
          EVENT_BUS_NAME: this.eventBus.eventBusName,
          AWS_REGION: this.region,
          PLUGIN_REGISTRY_TTL: "30",
          REDIS_URL: `redis://${redisEndpoint}:${redisPort}`,
        },
        secrets: {
          DATABASE_URL: ecs.Secret.fromSecretsManager(
            props.dbSecret,
            "password",
          ),
        },
        logDriver: ecs.LogDrivers.awsLogs({
          streamPrefix: "core",
          logRetention: logs.RetentionDays.ONE_WEEK,
        }),
      },
      healthCheckGracePeriod: Duration.seconds(60),
    });

    this.coreService.targetGroup.configureHealthCheck({
      path: "/health",
      interval: Duration.seconds(15),
      healthyHttpCodes: "200",
    });

    // Allow core service to talk to Redis (defined in this stack to avoid cycles)
    new ec2.CfnSecurityGroupIngress(this, "RedisFromCoreIngress", {
      groupId: props.redisSecurityGroup.securityGroupId,
      ipProtocol: "tcp",
      fromPort: 6379,
      toPort: 6379,
      sourceSecurityGroupId:
        this.coreService.service.connections.securityGroups[0]!.securityGroupId,
      description: "Core ECS service",
    });

    // Allow core service to talk to Aurora (same approach)
    new ec2.CfnSecurityGroupIngress(this, "AuroraFromCoreIngress", {
      groupId: props.aurora.connections.securityGroups[0]!.securityGroupId,
      ipProtocol: "tcp",
      fromPort: 5432,
      toPort: 5432,
      sourceSecurityGroupId:
        this.coreService.service.connections.securityGroups[0]!.securityGroupId,
      description: "Core ECS service",
    });

    // Grant put-events permission on event bus
    this.eventBus.grantPutEventsTo(this.coreService.taskDefinition.taskRole);

    // REST API gateway in front of the ALB (HTTP proxy)
    this.restApi = new apigw.RestApi(this, "CoreApi", {
      restApiName: "msa2-core-api",
      deployOptions: { stageName: "prod" },
    });
    const proxy = this.restApi.root.addProxy({
      anyMethod: false,
      defaultIntegration: new apigw.HttpIntegration(
        `http://${this.coreService.loadBalancer.loadBalancerDnsName}/{proxy}`,
        {
          httpMethod: "ANY",
          options: {
            requestParameters: {
              "integration.request.path.proxy": "method.request.path.proxy",
            },
          },
          proxy: true,
        },
      ),
    });
    proxy.addMethod("ANY", undefined, {
      requestParameters: { "method.request.path.proxy": true },
    });
  }
}
