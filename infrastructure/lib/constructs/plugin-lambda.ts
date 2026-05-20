import { Duration } from "aws-cdk-lib";
import * as events from "aws-cdk-lib/aws-events";
import * as targets from "aws-cdk-lib/aws-events-targets";
import * as lambda from "aws-cdk-lib/aws-lambda";
import * as nodejs from "aws-cdk-lib/aws-lambda-nodejs";
import * as logs from "aws-cdk-lib/aws-logs";
import { Construct } from "constructs";

export interface PluginLambdaProps {
  pluginId: string;
  entry: string;
  handler?: string;
  eventBus: events.IEventBus;
  eventSubscriptions?: string[];
  environment?: Record<string, string>;
  timeout?: Duration;
  memorySize?: number;
}

export class PluginLambda extends Construct {
  public readonly fn: lambda.Function;

  constructor(scope: Construct, id: string, props: PluginLambdaProps) {
    super(scope, id);

    this.fn = new nodejs.NodejsFunction(this, "Function", {
      entry: props.entry,
      handler: props.handler ?? "handler",
      runtime: lambda.Runtime.NODEJS_20_X,
      timeout: props.timeout ?? Duration.seconds(10),
      memorySize: props.memorySize ?? 256,
      environment: {
        EVENT_BUS_NAME: props.eventBus.eventBusName,
        PLUGIN_ID: props.pluginId,
        ...(props.environment ?? {}),
      },
      logRetention: logs.RetentionDays.ONE_WEEK,
      bundling: { externalModules: ["@aws-sdk/*"], minify: true },
    });

    props.eventBus.grantPutEventsTo(this.fn);

    if (props.eventSubscriptions && props.eventSubscriptions.length > 0) {
      new events.Rule(this, "EventRule", {
        eventBus: props.eventBus,
        eventPattern: { detailType: props.eventSubscriptions },
        targets: [new targets.LambdaFunction(this.fn)],
      });
    }
  }
}
