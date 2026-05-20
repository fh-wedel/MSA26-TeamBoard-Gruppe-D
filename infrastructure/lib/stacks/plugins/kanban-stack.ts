import { Stack, StackProps } from "aws-cdk-lib";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as ecs from "aws-cdk-lib/aws-ecs";
import * as events from "aws-cdk-lib/aws-events";
import * as elasticache from "aws-cdk-lib/aws-elasticache";
import * as rds from "aws-cdk-lib/aws-rds";
import * as secretsmanager from "aws-cdk-lib/aws-secretsmanager";
import { Construct } from "constructs";
import { PluginEcs } from "../../constructs/plugin-ecs";

export interface KanbanStackProps extends StackProps {
  cluster: ecs.ICluster;
  vpc: ec2.IVpc;
  aurora: rds.DatabaseCluster;
  redis: elasticache.CfnReplicationGroup;
  redisSecurityGroup: ec2.SecurityGroup;
  dbSecret: secretsmanager.ISecret;
  eventBus: events.IEventBus;
  coreUrl: string;
  imageTag: string;
  ecrRepositoryName?: string;
}

export class KanbanStack extends Stack {
  public readonly plugin: PluginEcs;

  constructor(scope: Construct, id: string, props: KanbanStackProps) {
    super(scope, id, props);

    const image = props.ecrRepositoryName
      ? ecs.ContainerImage.fromRegistry(
          `${this.account}.dkr.ecr.${this.region}.amazonaws.com/${props.ecrRepositoryName}:${props.imageTag}`,
        )
      : ecs.ContainerImage.fromAsset("../services/plugins/kanban");

    const redisEndpoint = props.redis.attrPrimaryEndPointAddress;
    const redisPort = props.redis.attrPrimaryEndPointPort;

    this.plugin = new PluginEcs(this, "Plugin", {
      cluster: props.cluster,
      image,
      containerPort: 3001,
      pluginId: "kanban-board",
      coreUrl: props.coreUrl,
      eventBus: props.eventBus,
      environment: {
        KANBAN_PORT: "3001",
        KANBAN_PLUGIN_ID: "kanban-board",
        KANBAN_PLUGIN_VERSION: "1.0.0",
        REDIS_URL: `redis://${redisEndpoint}:${redisPort}`,
      },
      secrets: {
        DATABASE_URL: ecs.Secret.fromSecretsManager(props.dbSecret, "password"),
      },
    });

    // Plugin's public endpoint = its internal ALB DNS
    this.plugin.service.taskDefinition.defaultContainer?.addEnvironment(
      "KANBAN_PUBLIC_ENDPOINT",
      `http://${this.plugin.service.loadBalancer.loadBalancerDnsName}`,
    );

    new ec2.CfnSecurityGroupIngress(this, "RedisFromKanbanIngress", {
      groupId: props.redisSecurityGroup.securityGroupId,
      ipProtocol: "tcp",
      fromPort: 6379,
      toPort: 6379,
      sourceSecurityGroupId:
        this.plugin.service.service.connections.securityGroups[0]!.securityGroupId,
      description: "Kanban plugin ECS",
    });

    new ec2.CfnSecurityGroupIngress(this, "AuroraFromKanbanIngress", {
      groupId: props.aurora.connections.securityGroups[0]!.securityGroupId,
      ipProtocol: "tcp",
      fromPort: 5432,
      toPort: 5432,
      sourceSecurityGroupId:
        this.plugin.service.service.connections.securityGroups[0]!.securityGroupId,
      description: "Kanban plugin ECS",
    });
  }
}
