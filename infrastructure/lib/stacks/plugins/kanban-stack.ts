import { Stack, StackProps } from "aws-cdk-lib";
import * as dynamodb from "aws-cdk-lib/aws-dynamodb";
import * as ec2 from "aws-cdk-lib/aws-ec2";
import * as ecs from "aws-cdk-lib/aws-ecs";
import * as events from "aws-cdk-lib/aws-events";
import { Construct } from "constructs";
import { PluginEcs } from "../../constructs/plugin-ecs";

export interface KanbanStackProps extends StackProps {
  cluster: ecs.ICluster;
  vpc: ec2.IVpc;
  ticketsTable: dynamodb.ITable;
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
        STORAGE: "dynamodb",
        TICKETS_TABLE: props.ticketsTable.tableName,
        EVENT_BUS: "eventbridge",
        EVENT_BUS_NAME: props.eventBus.eventBusName,
        AWS_REGION: Stack.of(this).region,
      },
    });

    props.ticketsTable.grantReadWriteData(
      this.plugin.service.taskDefinition.taskRole,
    );

    // Plugin's public endpoint = its internal ALB DNS
    this.plugin.service.taskDefinition.defaultContainer?.addEnvironment(
      "KANBAN_PUBLIC_ENDPOINT",
      `http://${this.plugin.service.loadBalancer.loadBalancerDnsName}`,
    );
  }
}
