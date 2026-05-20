import { Duration } from "aws-cdk-lib";
import * as ecs from "aws-cdk-lib/aws-ecs";
import * as ecs_patterns from "aws-cdk-lib/aws-ecs-patterns";
import * as events from "aws-cdk-lib/aws-events";
import * as logs from "aws-cdk-lib/aws-logs";
import { Construct } from "constructs";

export interface PluginEcsProps {
  cluster: ecs.ICluster;
  image: ecs.ContainerImage;
  containerPort: number;
  pluginId: string;
  publicEndpoint?: string;
  coreUrl: string;
  eventBus: events.IEventBus;
  environment?: Record<string, string>;
  secrets?: Record<string, ecs.Secret>;
  cpu?: number;
  memoryMiB?: number;
  desiredCount?: number;
  publicLoadBalancer?: boolean;
}

export class PluginEcs extends Construct {
  public readonly service: ecs_patterns.ApplicationLoadBalancedFargateService;

  constructor(scope: Construct, id: string, props: PluginEcsProps) {
    super(scope, id);

    this.service = new ecs_patterns.ApplicationLoadBalancedFargateService(this, "Service", {
      cluster: props.cluster,
      cpu: props.cpu ?? 256,
      memoryLimitMiB: props.memoryMiB ?? 512,
      desiredCount: props.desiredCount ?? 1,
      publicLoadBalancer: props.publicLoadBalancer ?? false,
      taskImageOptions: {
        image: props.image,
        containerPort: props.containerPort,
        environment: {
          NODE_ENV: "production",
          EVENT_BUS: "eventbridge",
          EVENT_BUS_NAME: props.eventBus.eventBusName,
          CORE_URL: props.coreUrl,
          ...(props.publicEndpoint
            ? { KANBAN_PUBLIC_ENDPOINT: props.publicEndpoint }
            : {}),
          ...(props.environment ?? {}),
        },
        secrets: props.secrets,
        logDriver: ecs.LogDrivers.awsLogs({
          streamPrefix: `plugin-${props.pluginId}`,
          logRetention: logs.RetentionDays.ONE_WEEK,
        }),
      },
      healthCheckGracePeriod: Duration.seconds(60),
    });

    this.service.targetGroup.configureHealthCheck({
      path: "/health",
      interval: Duration.seconds(15),
      healthyHttpCodes: "200",
    });

    props.eventBus.grantPutEventsTo(this.service.taskDefinition.taskRole);
  }
}
