#!/usr/bin/env node
import "source-map-support/register";
import { App } from "aws-cdk-lib";
import { NetworkStack } from "../lib/stacks/network-stack";
import { PersistenceStack } from "../lib/stacks/persistence-stack";
import { CoreStack } from "../lib/stacks/core-stack";
import { RealtimeStack } from "../lib/stacks/realtime-stack";
import { KanbanStack } from "../lib/stacks/plugins/kanban-stack";

const app = new App();

const env = {
  account: process.env.CDK_DEFAULT_ACCOUNT,
  region: process.env.CDK_DEFAULT_REGION ?? "eu-central-1",
};

const environment = app.node.tryGetContext("environment") ?? "staging";
const imageTag = app.node.tryGetContext("imageTag") ?? "latest";
const prefix = `Msa2-${capitalize(environment)}`;

const network = new NetworkStack(app, `${prefix}-Network`, { env });

const persistence = new PersistenceStack(app, `${prefix}-Persistence`, {
  env,
  vpc: network.vpc,
});
persistence.addDependency(network);

const core = new CoreStack(app, `${prefix}-Core`, {
  env,
  vpc: network.vpc,
  aurora: persistence.aurora,
  redis: persistence.redis,
  redisSecurityGroup: persistence.redisSecurityGroup,
  dbSecret: persistence.dbSecret,
  imageTag,
});
core.addDependency(persistence);

const realtime = new RealtimeStack(app, `${prefix}-Realtime`, {
  env,
  eventBus: core.eventBus,
  connectionsTable: persistence.connectionsTable,
});
realtime.addDependency(core);

const kanban = new KanbanStack(app, `${prefix}-PluginKanban`, {
  env,
  cluster: core.cluster,
  vpc: network.vpc,
  aurora: persistence.aurora,
  redis: persistence.redis,
  redisSecurityGroup: persistence.redisSecurityGroup,
  dbSecret: persistence.dbSecret,
  eventBus: core.eventBus,
  coreUrl: `http://${core.coreService.loadBalancer.loadBalancerDnsName}`,
  imageTag,
});
kanban.addDependency(core);

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

app.synth();
