import { Duration, Stack, StackProps } from "aws-cdk-lib";
import * as apigw from "aws-cdk-lib/aws-apigateway";
import * as dynamodb from "aws-cdk-lib/aws-dynamodb";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as ecs from "aws-cdk-lib/aws-ecs";
import * as ecs_patterns from "aws-cdk-lib/aws-ecs-patterns";
import * as events from "aws-cdk-lib/aws-events";
import * as logs from "aws-cdk-lib/aws-logs";
import { Construct } from "constructs";

export interface CoreStackProps extends StackProps {
  vpc: ec2.IVpc;
  pluginRegistryTable: dynamodb.ITable;
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
          REGISTRY_BACKEND: "dynamodb",
          PLUGIN_REGISTRY_TABLE: props.pluginRegistryTable.tableName,
          PLUGIN_REGISTRY_TTL: "30",
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

    props.pluginRegistryTable.grantReadWriteData(
      this.coreService.taskDefinition.taskRole,
    );
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
