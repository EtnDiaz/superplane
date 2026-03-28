import type {
  ComponentBaseContext,
  ComponentBaseMapper,
  EventStateRegistry,
  ExecutionDetailsContext,
  ExecutionInfo,
  NodeInfo,
  SubtitleContext,
} from "./types";
import type { ComponentBaseProps, EventSection, EventState, EventStateMap } from "@/ui/componentBase";
import { DEFAULT_EVENT_STATE_MAP } from "@/ui/componentBase";
import { getColorClass } from "@/utils/colors";
import type React from "react";
import { getTriggerRenderer } from ".";
import { renderTimeAgo } from "@/components/TimeAgo";

const SANDBOX_STATE_MAP: EventStateMap = {
  ...DEFAULT_EVENT_STATE_MAP,
};

const sandboxStateFunction = (execution: ExecutionInfo): EventState => {
  if (!execution) return "neutral";

  if (
    execution.resultMessage &&
    (execution.resultReason === "RESULT_REASON_ERROR" ||
      (execution.result === "RESULT_FAILED" && execution.resultReason !== "RESULT_REASON_ERROR_RESOLVED"))
  ) {
    return "error";
  }

  if (execution.result === "RESULT_CANCELLED") return "cancelled";

  if (execution.state === "STATE_PENDING" || execution.state === "STATE_STARTED") return "running";

  if (execution.state === "STATE_FINISHED" && execution.result === "RESULT_PASSED") return "success";

  return "neutral";
};

export const SANDBOX_STATE_REGISTRY: EventStateRegistry = {
  stateMap: SANDBOX_STATE_MAP,
  getState: sandboxStateFunction,
};

export const sandboxMapper: ComponentBaseMapper = {
  props(context: ComponentBaseContext): ComponentBaseProps {
    const lastExecution = context.lastExecutions.length > 0 ? context.lastExecutions[0] : null;

    return {
      iconSlug: context.componentDefinition?.icon ?? "shield",
      collapsedBackground: getColorClass(context.componentDefinition?.color ?? "gray"),
      collapsed: context.node.isCollapsed,
      title: context.node.name || context.componentDefinition?.label || "Sandbox",
      eventSections: lastExecution
        ? buildEventSections(context.nodes, lastExecution)
        : undefined,
      includeEmptyState: !lastExecution,
      eventStateMap: SANDBOX_STATE_MAP,
    };
  },

  getExecutionDetails(context: ExecutionDetailsContext): Record<string, string> {
    const details: Record<string, string> = {};
    const outputs = context.execution.outputs as { default?: Array<{ data?: Record<string, unknown> }> } | undefined;
    const payload = outputs?.default?.[0];

    if (payload?.data?.provider) details["Provider"] = String(payload.data.provider);
    if (payload?.data?.durationMs !== undefined) details["Duration"] = `${String(payload.data.durationMs)}ms`;
    if (payload?.data?.exitCode !== undefined) details["Exit Code"] = String(payload.data.exitCode);

    return details;
  },

  subtitle(context: SubtitleContext): string | React.ReactNode {
    const timestamp = context.execution.updatedAt || context.execution.createdAt;
    return timestamp ? renderTimeAgo(new Date(timestamp)) : "";
  },
};

function buildEventSections(nodes: NodeInfo[], execution: ExecutionInfo): EventSection[] {
  const rootTriggerNode = nodes.find((n) => n.id === execution.rootEvent?.nodeId);
  const rootTriggerRenderer = getTriggerRenderer(rootTriggerNode?.componentName ?? "");
  const { title } = rootTriggerRenderer.getTitleAndSubtitle({ event: execution.rootEvent });
  const subtitleTimestamp = execution.updatedAt || execution.createdAt;

  return [
    {
      receivedAt: new Date(execution.createdAt!),
      eventTitle: title,
      eventSubtitle: subtitleTimestamp ? renderTimeAgo(new Date(subtitleTimestamp)) : "",
      eventState: sandboxStateFunction(execution),
      eventId: execution.rootEvent!.id!,
    },
  ];
}
