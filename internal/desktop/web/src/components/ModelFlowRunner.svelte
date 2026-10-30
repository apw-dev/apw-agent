<script lang="ts">
  import { Eye, Sparkles, AlertCircle, CheckCircle2, RotateCcw, GripVertical } from "lucide-svelte";
  import type { TaskJob, TaskEvent, PromptTrace, RouteCandidateInfo } from "../lib/api";
  import { extractRouteCandidates, extractTaskPromptTraces, ACTIVE_TASK_STATUSES } from "../lib/stores/tasks";
  import { t } from "../lib/i18n";

  let {
    job,
    events = [],
    onInspectPrompt
  }: {
    job: TaskJob;
    events?: TaskEvent[];
    onInspectPrompt: (trace: PromptTrace) => void;
  } = $props();

  const isTaskRunning = $derived(job ? ACTIVE_TASK_STATUSES.has(job.status) : false);
  const promptTraces = $derived(extractTaskPromptTraces(events));
  const rawCandidates = $derived(extractRouteCandidates(events));

  // Derive model route candidates from live route_stack events or prompt traces
  const candidates = $derived.by((): RouteCandidateInfo[] => {
    if (rawCandidates.length > 0) return rawCandidates.slice(0, 4);
    if (promptTraces.length > 0) {
      const seen = new Set<string>();
      const list: RouteCandidateInfo[] = [];
      for (const tr of promptTraces) {
        if (!seen.has(tr.model)) {
          seen.add(tr.model);
          list.push({
            provider_key: `${tr.provider}:${tr.model}`,
            provider: tr.provider,
            model: tr.model,
            priority: list.length + 1,
            role: list.length === 0 ? "primary" : "fallback",
            status: tr.status === "running" ? "active" : tr.status === "failed" ? "failed" : "ready"
          });
        }
      }
      return list.slice(0, 4);
    }
    return [
      {
        provider_key: "primary",
        provider: "agy",
        model: "claude-sonnet-5-5-high",
        priority: 1,
        role: "primary",
        status: isTaskRunning ? "active" : "ready"
      },
      {
        provider_key: "fallback-1",
        provider: "agy",
        model: "claude-opus-5-5-high",
        priority: 2,
        role: "fallback",
        status: "ready"
      }
    ];
  });

  function getCandidateTrace(modelName: string): PromptTrace | undefined {
    for (let i = promptTraces.length - 1; i >= 0; i--) {
      if (promptTraces[i].model === modelName) {
        return promptTraces[i];
      }
    }
    return undefined;
  }

  const activeCandidateIndex = $derived.by((): number => {
    for (let i = 0; i < candidates.length; i++) {
      const tr = getCandidateTrace(candidates[i].model);
      if (tr && tr.status === "running") return i;
    }
    if (promptTraces.length > 0) {
      const lastModel = promptTraces[promptTraces.length - 1].model;
      const idx = candidates.findIndex((c) => c.model === lastModel);
      if (idx >= 0) return idx;
    }
    return isTaskRunning ? 0 : -1;
  });

  // XYFlow Node Dragging State
  let nodeOffsets = $state<Record<string, { dx: number; dy: number }>>({});
  let draggingNodeId = $state<string | null>(null);
  let dragStartPointer = $state<{ x: number; y: number }>({ x: 0, y: 0 });
  let dragStartOffset = $state<{ dx: number; dy: number }>({ dx: 0, dy: 0 });

  function getOffset(id: string): { dx: number; dy: number } {
    return nodeOffsets[id] ?? { dx: 0, dy: 0 };
  }

  function resetLayout() {
    nodeOffsets = {};
  }

  function startNodeDrag(e: PointerEvent, id: string) {
    const target = e.target as HTMLElement;
    if (target.closest("button")) return;
    e.stopPropagation();
    draggingNodeId = id;
    dragStartPointer = { x: e.clientX, y: e.clientY };
    const cur = getOffset(id);
    dragStartOffset = { dx: cur.dx, dy: cur.dy };
    (e.currentTarget as HTMLElement)?.setPointerCapture?.(e.pointerId);
  }

  function moveNodeDrag(e: PointerEvent) {
    if (!draggingNodeId) return;
    const dx = dragStartOffset.dx + (e.clientX - dragStartPointer.x);
    const dy = dragStartOffset.dy + (e.clientY - dragStartPointer.y);
    nodeOffsets = {
      ...nodeOffsets,
      [draggingNodeId]: { dx, dy }
    };
  }

  function endNodeDrag() {
    draggingNodeId = null;
  }

  // Dynamic XYFlow Canvas Geometry
  const ROUTER_W = 184;
  const ROUTER_H = 104;
  const CAND_W = 282;
  const CAND_H = 72;
  const CAND_GAP = 26;
  const TRACE_W = 236;
  const TRACE_H = 54;
  const TRACE_GAP = 14;

  const visibleTraces = $derived(promptTraces.slice(-6));

  const canvasHeight = $derived.by((): number => {
    const candStackH = candidates.length * CAND_H + Math.max(0, candidates.length - 1) * CAND_GAP;
    const traceStackH = visibleTraces.length * TRACE_H + Math.max(0, visibleTraces.length - 1) * TRACE_GAP;
    return Math.max(228, candStackH + 48, traceStackH + 44);
  });

  // Router Node Coordinates
  const routerNode = $derived.by(() => {
    const off = getOffset("router");
    const x = 22 + off.dx;
    const y = Math.round((canvasHeight - ROUTER_H) / 2) + off.dy;
    return {
      id: "router",
      x,
      y,
      w: ROUTER_W,
      h: ROUTER_H,
      rightHandle: { x: x + ROUTER_W, y: y + Math.round(ROUTER_H / 2) }
    };
  });

  // Candidate Route Nodes Coordinates
  const candidateNodes = $derived.by(() => {
    const totalH = candidates.length * CAND_H + Math.max(0, candidates.length - 1) * CAND_GAP;
    const startY = Math.round((canvasHeight - totalH) / 2);
    return candidates.map((cand, idx) => {
      const id = `cand-${idx}`;
      const off = getOffset(id);
      const x = 262 + off.dx;
      const y = startY + idx * (CAND_H + CAND_GAP) + off.dy;
      const tr = getCandidateTrace(cand.model);
      const isActive = idx === activeCandidateIndex && isTaskRunning;
      const isFailed = tr?.status === "failed" || tr?.status === "fallback" || cand.status === "failed";
      const isDone = tr?.status === "completed";
      return {
        id,
        idx,
        cand,
        tr,
        isActive,
        isFailed,
        isDone,
        x,
        y,
        w: CAND_W,
        h: CAND_H,
        leftHandle: { x, y: y + Math.round(CAND_H / 2) },
        rightHandle: { x: x + CAND_W, y: y + Math.round(CAND_H / 2) },
        topHandle: { x: x + Math.round(CAND_W / 2), y },
        bottomHandle: { x: x + Math.round(CAND_W / 2), y: y + CAND_H }
      };
    });
  });

  // Prompt Pipeline Step Nodes Coordinates (Column 3)
  const traceNodes = $derived.by(() => {
    if (visibleTraces.length === 0) return [];
    const totalH = visibleTraces.length * TRACE_H + Math.max(0, visibleTraces.length - 1) * TRACE_GAP;
    const startY = Math.round((canvasHeight - totalH) / 2);
    return visibleTraces.map((tr, idx) => {
      const globalIdx = promptTraces.length - visibleTraces.length + idx;
      const id = `trace-${globalIdx}`;
      const off = getOffset(id);
      const x = 606 + off.dx;
      const y = startY + idx * (TRACE_H + TRACE_GAP) + off.dy;
      const candIdx = candidates.findIndex((c) => c.model === tr.model);
      const sourceCandIdx = candIdx >= 0 ? candIdx : 0;
      return {
        id,
        globalIdx,
        tr,
        sourceCandIdx,
        x,
        y,
        w: TRACE_W,
        h: TRACE_H,
        leftHandle: { x, y: y + Math.round(TRACE_H / 2) }
      };
    });
  });

  // Compute exact XYFlow Horizontal Cubic Bezier Path from (sx, sy) to (tx, ty)
  function bezierHorizontal(sx: number, sy: number, tx: number, ty: number) {
    const dx = Math.abs(tx - sx);
    const pull = Math.max(dx * 0.48, 36);
    const cpx1 = sx + pull;
    const cpy1 = sy;
    const cpx2 = tx - pull;
    const cpy2 = ty;
    const mx = 0.125 * sx + 0.375 * cpx1 + 0.375 * cpx2 + 0.125 * tx;
    const my = 0.125 * sy + 0.375 * cpy1 + 0.375 * cpy2 + 0.125 * ty;
    return {
      d: `M ${sx} ${sy} C ${cpx1} ${cpy1}, ${cpx2} ${cpy2}, ${tx} ${ty}`,
      sx,
      sy,
      tx,
      ty,
      mx,
      my
    };
  }

  // Compute exact XYFlow Vertical Cubic Bezier Path for failover cascade
  function bezierVertical(sx: number, sy: number, tx: number, ty: number) {
    const dy = Math.abs(ty - sy);
    const pull = Math.max(dy * 0.45, 12);
    const cpx1 = sx;
    const cpy1 = sy + pull;
    const cpx2 = tx;
    const cpy2 = ty - pull;
    const mx = 0.125 * sx + 0.375 * cpx1 + 0.375 * cpx2 + 0.125 * tx;
    const my = 0.125 * sy + 0.375 * cpy1 + 0.375 * cpy2 + 0.125 * ty;
    return {
      d: `M ${sx} ${sy} C ${cpx1} ${cpy1}, ${cpx2} ${cpy2}, ${tx} ${ty}`,
      sx,
      sy,
      tx,
      ty,
      mx,
      my
    };
  }
</script>

<div class="flow-runner-container" data-testid="model-flow-runner">
  <div class="flow-header">
    <div class="header-left">
      <Sparkles size={14} class="text-accent" />
      <span class="header-title">Live Model Route Flow</span>
      <span class="strategy-badge mono">XYFLOW · PRIORITY CASCADE</span>
    </div>
    <div class="header-right">
      {#if Object.keys(nodeOffsets).length > 0}
        <button
          type="button"
          class="xy-reset-btn mono"
          onclick={resetLayout}
          title={$t("Reset XYFlow node positions", "รีเซ็ตตำแหน่งโหนด XYFlow")}
        >
          <RotateCcw size={11} />
          <span>Reset Layout</span>
        </button>
      {/if}
      {#if promptTraces.length > 0}
        <button
          type="button"
          class="inspect-all-btn mono"
          onclick={() => onInspectPrompt(promptTraces[promptTraces.length - 1])}
          title={$t("Open Prompt Inspector for latest trace", "เปิด Prompt Inspector สำหรับคำสั่งล่าสุด")}
        >
          <Eye size={12} />
          <span>{$t(`Inspect Latest Prompt (${promptTraces.length})`, `ดู Prompt ล่าสุด (${promptTraces.length} ครั้ง)`)}</span>
        </button>
      {/if}
    </div>
  </div>

  <!-- XYFlow Interactive Node-Based Viewport -->
  <div
    class="xyflow-runner-viewport"
    role="application"
    style={`height: ${canvasHeight}px;`}
    onpointermove={moveNodeDrag}
    onpointerup={endNodeDrag}
    onpointercancel={endNodeDrag}
  >
    <!-- SVG Bezier Connection Edges Layer (100% aligned to node handle coordinates) -->
    <svg
      class="xyflow-edges-layer"
      viewBox={`0 0 864 ${canvasHeight}`}
      preserveAspectRatio="xMinYMid meet"
      xmlns="http://www.w3.org/2000/svg"
    >
      <defs>
        <linearGradient id="xyEdgeActive" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stop-color="#8b5cf6" />
          <stop offset="100%" stop-color="#22c55e" />
        </linearGradient>
        <linearGradient id="xyEdgeDone" x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stop-color="#6366f1" />
          <stop offset="100%" stop-color="#10b981" />
        </linearGradient>
      </defs>

      <!-- 1. Router Right Handle -> Candidate Left Handle Edges -->
      {#each candidateNodes as cNode (cNode.id)}
        {@const edge = bezierHorizontal(
          routerNode.rightHandle.x,
          routerNode.rightHandle.y,
          cNode.leftHandle.x,
          cNode.leftHandle.y
        )}
        {@const edgeId = `xy-router-to-${cNode.id}-${job?.id || "default"}`}
        {@const isHighlighted = cNode.isActive || cNode.isDone || cNode.idx === activeCandidateIndex}
        <g class="xy-edge-group">
          {#if isHighlighted}
            <path
              d={edge.d}
              fill="none"
              stroke={cNode.isActive ? "#22c55e" : "#8b5cf6"}
              stroke-width="5"
              opacity="0.14"
            />
          {/if}
          <path
            id={edgeId}
            d={edge.d}
            fill="none"
            stroke={cNode.isActive
              ? "url(#xyEdgeActive)"
              : cNode.isDone
                ? "url(#xyEdgeDone)"
                : cNode.isFailed
                  ? "#ef4444"
                  : "#6366f1"}
            stroke-width={isHighlighted ? "2.2" : "1.5"}
            stroke-dasharray={cNode.isDone || cNode.isActive ? "none" : "4 4"}
            opacity={isHighlighted ? "0.95" : "0.38"}
          />
          {#if isTaskRunning && cNode.idx === activeCandidateIndex}
            <circle r="3.5" fill="#22c55e">
              <animateMotion dur="1.15s" repeatCount="indefinite">
                <mpath href={`#${edgeId}`} />
              </animateMotion>
            </circle>
            <circle r="3.5" fill="#a78bfa">
              <animateMotion dur="1.15s" begin="0.55s" repeatCount="indefinite">
                <mpath href={`#${edgeId}`} />
              </animateMotion>
            </circle>
          {/if}
        </g>
      {/each}

      <!-- 2. Candidate Bottom Handle -> Next Candidate Top Handle (Failover Cascade) -->
      {#each candidateNodes as cNode, idx (cNode.id)}
        {#if idx < candidateNodes.length - 1}
          {@const nextNode = candidateNodes[idx + 1]}
          {@const dropEdge = bezierVertical(
            cNode.bottomHandle.x,
            cNode.bottomHandle.y,
            nextNode.topHandle.x,
            nextNode.topHandle.y
          )}
          {@const dropId = `xy-drop-${idx}-${job?.id || "default"}`}
          {@const didFailover = cNode.isFailed || activeCandidateIndex > idx}
          <g class="xy-edge-group">
            <path
              id={dropId}
              d={dropEdge.d}
              fill="none"
              stroke="#ef4444"
              stroke-width={didFailover ? "1.8" : "1.3"}
              stroke-dasharray="3 3"
              opacity={didFailover ? "0.85" : "0.35"}
            />
            <g transform={`translate(${dropEdge.mx}, ${dropEdge.my})`}>
              <rect x="-36" y="-8" width="72" height="15" rx="4" fill="#131722" stroke="rgba(239,68,68,0.35)" stroke-width="1" />
              <text x="0" y="2.5" text-anchor="middle" class="xy-drop-label">fails ➔ next</text>
            </g>
            {#if isTaskRunning && didFailover}
              <circle r="3" fill="#ef4444">
                <animateMotion dur="1.2s" repeatCount="indefinite">
                  <mpath href={`#${dropId}`} />
                </animateMotion>
              </circle>
            {/if}
          </g>
        {/if}
      {/each}

      <!-- 3. Candidate Right Handle -> Prompt Step Node Left Handle Edges -->
      {#each traceNodes as tNode (tNode.id)}
        {@const sourceCand = candidateNodes[tNode.sourceCandIdx] ?? candidateNodes[0]}
        {#if sourceCand}
          {@const traceEdge = bezierHorizontal(
            sourceCand.rightHandle.x,
            sourceCand.rightHandle.y,
            tNode.leftHandle.x,
            tNode.leftHandle.y
          )}
          {@const traceEdgeId = `xy-trace-edge-${tNode.id}-${job?.id || "default"}`}
          {@const isTraceRunning = tNode.tr.status === "running"}
          {@const isTraceFailed = tNode.tr.status === "failed"}
          <g class="xy-edge-group">
            <path
              id={traceEdgeId}
              d={traceEdge.d}
              fill="none"
              stroke={isTraceRunning
                ? "#22c55e"
                : isTraceFailed
                  ? "#ef4444"
                  : "#8b5cf6"}
              stroke-width={isTraceRunning ? "2" : "1.5"}
              stroke-dasharray={isTraceRunning ? "4 3" : "none"}
              opacity={isTraceRunning ? "0.95" : "0.6"}
            />
            {#if isTraceRunning}
              <circle r="3.2" fill="#22c55e">
                <animateMotion dur="1s" repeatCount="indefinite">
                  <mpath href={`#${traceEdgeId}`} />
                </animateMotion>
              </circle>
            {/if}
          </g>
        {/if}
      {/each}
    </svg>

    <!-- HTML XYFlow Node Layer -->
    <div class="xyflow-nodes-layer">
      <!-- Column 1: Router / Dispatcher XYNode -->
      <div
        class="xy-node router-xy-node"
        class:running={isTaskRunning}
        style={`left: ${routerNode.x}px; top: ${routerNode.y}px; width: ${routerNode.w}px; height: ${routerNode.h}px;`}
        onpointerdown={(e) => startNodeDrag(e, "router")}
        role="group"
        aria-label="Router Dispatcher Node"
      >
        <div class="xy-node-top">
          <span class="combo-label mono">ROUTER</span>
          <GripVertical size={12} class="xy-grip" />
        </div>
        <span class="combo-name mono">"always-on"</span>
        <span class="combo-desc mono">try #1 first · failover cascade</span>
        <div class="combo-status">
          <span class="status-indicator" class:pulse={isTaskRunning}></span>
          <span class="status-text mono">{isTaskRunning ? "DISPATCHING" : "IDLE / READY"}</span>
        </div>
        <!-- Exact Right Handle Port -->
        <span class="xy-port right" title="Router Output Handle"></span>
      </div>

      <!-- Column 2: Candidate Route XYNodes -->
      {#each candidateNodes as cNode (cNode.id)}
        <div
          class="xy-node candidate-xy-node"
          class:active={cNode.isActive}
          class:failed={cNode.isFailed}
          class:done={cNode.isDone}
          style={`left: ${cNode.x}px; top: ${cNode.y}px; width: ${cNode.w}px; height: ${cNode.h}px;`}
          onpointerdown={(e) => startNodeDrag(e, cNode.id)}
          onclick={() => cNode.tr && onInspectPrompt(cNode.tr)}
          role="button"
          tabindex="0"
          onkeydown={(e) => e.key === "Enter" && cNode.tr && onInspectPrompt(cNode.tr)}
        >
          <!-- Exact Left & Right Handles + Top/Bottom Failover Handles -->
          <span class="xy-port left" title="Route Input Handle"></span>
          {#if visibleTraces.length > 0}
            <span class="xy-port right" title="Prompt Stream Output Handle"></span>
          {/if}
          {#if cNode.idx > 0}
            <span class="xy-port top" title="Failover Input Handle"></span>
          {/if}
          {#if cNode.idx < candidateNodes.length - 1}
            <span class="xy-port bottom" title="Failover Output Handle"></span>
          {/if}

          <div class="card-left">
            <span class="priority-num mono">{cNode.idx + 1}</span>
            <div class="model-info">
              <div class="model-title-row">
                <span class="provider-prefix mono">{cNode.cand.provider}/</span>
                <span class="model-id mono">{cNode.cand.model}</span>
              </div>
              <div class="model-sub mono">
                <span class="role-text">
                  {cNode.cand.role === "primary" ? "Primary Route" : `Fallback Layer #${cNode.idx}`}
                </span>
                {#if cNode.tr?.duration_ms}
                  <span class="meta-dot">·</span>
                  <span class="meta-time">⏱ {(cNode.tr.duration_ms / 1000).toFixed(1)}s</span>
                {/if}
              </div>
            </div>
          </div>

          <div class="card-right">
            {#if cNode.isActive}
              <div class="state-pill active mono">
                <span class="pulse-dot"></span>
                <span>RUNNING</span>
              </div>
            {:else if cNode.isFailed}
              <div class="state-pill failed mono" title={cNode.tr?.error || "Failed / Quota"}>
                <AlertCircle size={11} />
                <span>FAILED</span>
              </div>
            {:else if cNode.isDone}
              <div class="state-pill done mono">
                <CheckCircle2 size={11} />
                <span>DONE</span>
              </div>
            {:else}
              <div class="state-pill standby mono">
                <span>STANDBY</span>
              </div>
            {/if}

            {#if cNode.tr}
              <button
                type="button"
                class="inspect-btn mono"
                onclick={(e) => {
                  e.stopPropagation();
                  if (cNode.tr) onInspectPrompt(cNode.tr);
                }}
                title="คลิกเพื่อดู Prompt ที่ส่งออกไปยังโมเดลนี้"
              >
                <Eye size={11} />
                <span>Prompt</span>
              </button>
            {/if}
          </div>
        </div>
      {/each}

      <!-- Column 3: Live Prompt Pipeline Step XYNodes -->
      {#each traceNodes as tNode (tNode.id)}
        <div
          class="xy-node trace-xy-node mono"
          class:running={tNode.tr.status === "running"}
          class:failed={tNode.tr.status === "failed"}
          class:done={tNode.tr.status === "completed"}
          style={`left: ${tNode.x}px; top: ${tNode.y}px; width: ${tNode.w}px; height: ${tNode.h}px;`}
          onpointerdown={(e) => startNodeDrag(e, tNode.id)}
          onclick={() => onInspectPrompt(tNode.tr)}
          role="button"
          tabindex="0"
          onkeydown={(e) => e.key === "Enter" && onInspectPrompt(tNode.tr)}
          title={`คลิกเพื่อเปิด Prompt #${tNode.globalIdx + 1}: ${tNode.tr.stage} (${tNode.tr.model})`}
        >
          <span class="xy-port left" title="Prompt Input Handle"></span>
          <div class="trace-node-top">
            <span class="trace-step-badge">#{tNode.globalIdx + 1}</span>
            <span class="trace-stage-name">{tNode.tr.stage}</span>
            {#if tNode.tr.duration_ms}
              <span class="trace-dur">{(tNode.tr.duration_ms / 1000).toFixed(1)}s</span>
            {/if}
          </div>
          <div class="trace-node-sub">
            <span class="trace-model-name">{tNode.tr.model}</span>
            <span class="trace-inspect-link">
              <Eye size={10} />
              <span>Inspect</span>
            </span>
          </div>
        </div>
      {/each}
    </div>
  </div>

  {#if promptTraces.length > 0}
    <div class="prompt-chain-strip mono">
      <span class="chain-title">Prompt Pipeline ({promptTraces.length}):</span>
      <div class="chain-pills">
        {#each promptTraces as tr, idx (tr.request_id ? `${tr.request_id}-${idx}` : idx)}
          <button
            type="button"
            class="chain-pill"
            class:running={tr.status === "running"}
            class:failed={tr.status === "failed"}
            onclick={() => onInspectPrompt(tr)}
            title={`คลิกเพื่อดู Prompt #${idx + 1} (${tr.stage} · ${tr.model})`}
          >
            <span class="chain-num">#{idx + 1}</span>
            <span class="chain-stage">{tr.stage}</span>
            <span class="chain-model">{tr.model}</span>
            {#if tr.duration_ms}
              <span class="chain-dur">{(tr.duration_ms / 1000).toFixed(1)}s</span>
            {/if}
          </button>
        {/each}
      </div>
    </div>
  {/if}
</div>

<style>
  .flow-runner-container {
    background: #0b0e14;
    border: 1px solid rgba(255, 255, 255, 0.08);
    border-radius: 12px;
    overflow: hidden;
    margin-bottom: 14px;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  }

  .flow-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 16px;
    background: #10141d;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    gap: 10px;
    flex-wrap: wrap;
  }

  .header-left,
  .header-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  :global(.text-accent) {
    color: #a78bfa;
  }

  .header-title {
    font-size: 13px;
    font-weight: 700;
    color: #f1f5f9;
  }

  .strategy-badge {
    font-size: 9.5px;
    letter-spacing: 0.8px;
    font-weight: 700;
    color: #a78bfa;
    background: rgba(139, 92, 246, 0.14);
    border: 1px solid rgba(139, 92, 246, 0.3);
    padding: 2px 7px;
    border-radius: 4px;
  }

  .xy-reset-btn {
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.12);
    color: #cbd5e1;
    font-size: 10.5px;
    padding: 4px 8px;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
  }

  .xy-reset-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #fff;
  }

  .inspect-all-btn {
    background: rgba(139, 92, 246, 0.14);
    border: 1px solid rgba(139, 92, 246, 0.38);
    color: #ddd6fe;
    font-size: 11px;
    padding: 4px 10px;
    border-radius: 6px;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .inspect-all-btn:hover {
    background: rgba(139, 92, 246, 0.28);
    color: #fff;
  }

  /* XYFlow Dotted Interactive Viewport */
  .xyflow-runner-viewport {
    position: relative;
    width: 100%;
    overflow-x: auto;
    overflow-y: hidden;
    background-color: #0b0e15;
    background-image:
      radial-gradient(circle at 14% 50%, rgba(139, 92, 246, 0.09) 0%, transparent 55%),
      radial-gradient(rgba(148, 163, 184, 0.16) 1px, transparent 1px);
    background-size: 100% 100%, 18px 18px;
    user-select: none;
  }

  .xyflow-edges-layer {
    position: absolute;
    top: 0;
    left: 0;
    width: 864px;
    height: 100%;
    pointer-events: none;
    z-index: 1;
  }

  .xy-drop-label {
    font-family: var(--font-mono);
    font-size: 8.5px;
    fill: #fca5a5;
    letter-spacing: 0.3px;
  }

  .xyflow-nodes-layer {
    position: relative;
    width: 864px;
    height: 100%;
    z-index: 2;
  }

  /* Base XYFlow Node */
  .xy-node {
    position: absolute;
    box-sizing: border-box;
    border-radius: 10px;
    background: #131824;
    border: 1px solid rgba(255, 255, 255, 0.1);
    box-shadow: 0 6px 18px rgba(0, 0, 0, 0.42);
    cursor: grab;
    transition: border-color 0.18s ease, box-shadow 0.18s ease;
  }

  .xy-node:active {
    cursor: grabbing;
  }

  /* XYFlow Connection Handle Ports */
  .xy-port {
    position: absolute;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: #1e293b;
    border: 2px solid #8b5cf6;
    box-shadow: 0 0 0 2px rgba(11, 14, 21, 0.9);
    z-index: 4;
    pointer-events: none;
  }

  .xy-port.left {
    left: -6px;
    top: calc(50% - 5px);
  }

  .xy-port.right {
    right: -6px;
    top: calc(50% - 5px);
  }

  .xy-port.top {
    top: -6px;
    left: calc(50% - 5px);
    border-color: #ef4444;
  }

  .xy-port.bottom {
    bottom: -6px;
    left: calc(50% - 5px);
    border-color: #ef4444;
  }

  /* Router XYNode */
  .router-xy-node {
    padding: 12px 14px;
    border-color: rgba(139, 92, 246, 0.45);
    display: flex;
    flex-direction: column;
    justify-content: space-between;
  }

  .router-xy-node.running {
    border-color: rgba(34, 197, 94, 0.55);
    box-shadow: 0 0 20px rgba(34, 197, 94, 0.12), 0 6px 18px rgba(0, 0, 0, 0.45);
  }

  .xy-node-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  :global(.xy-grip) {
    color: #475569;
  }

  .combo-label {
    font-size: 9px;
    letter-spacing: 1.8px;
    font-weight: 700;
    color: #a78bfa;
  }

  .combo-name {
    font-size: 13.5px;
    font-weight: 700;
    color: #f8fafc;
  }

  .combo-desc {
    font-size: 9.5px;
    color: #94a3b8;
    line-height: 1.3;
  }

  .combo-status {
    display: flex;
    align-items: center;
    gap: 6px;
    padding-top: 6px;
    border-top: 1px solid rgba(255, 255, 255, 0.06);
  }

  .status-indicator {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #64748b;
  }

  .status-indicator.pulse {
    background: #22c55e;
    box-shadow: 0 0 8px #22c55e;
    animation: pulseGlow 1.5s infinite;
  }

  .status-text {
    font-size: 9.5px;
    font-weight: 700;
    color: #cbd5e1;
  }

  /* Candidate Route XYNode */
  .candidate-xy-node {
    padding: 10px 12px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .candidate-xy-node:hover {
    border-color: rgba(139, 92, 246, 0.5);
  }

  .candidate-xy-node.active {
    background: rgba(34, 197, 94, 0.07);
    border-color: rgba(34, 197, 94, 0.55);
    box-shadow: 0 0 18px rgba(34, 197, 94, 0.14);
  }

  .candidate-xy-node.active .xy-port.left,
  .candidate-xy-node.active .xy-port.right {
    border-color: #22c55e;
    background: #052e16;
  }

  .candidate-xy-node.done {
    border-color: rgba(16, 185, 129, 0.4);
  }

  .candidate-xy-node.failed {
    background: rgba(239, 68, 68, 0.06);
    border-color: rgba(239, 68, 68, 0.45);
  }

  .card-left {
    display: flex;
    align-items: center;
    gap: 9px;
    min-width: 0;
    flex: 1;
  }

  .priority-num {
    width: 21px;
    height: 21px;
    border-radius: 6px;
    background: rgba(139, 92, 246, 0.16);
    border: 1px solid rgba(139, 92, 246, 0.35);
    color: #c4b5fd;
    font-size: 11px;
    font-weight: 700;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .model-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .model-title-row {
    display: flex;
    align-items: center;
    font-size: 11.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .provider-prefix {
    color: #94a3b8;
  }

  .model-id {
    color: #f1f5f9;
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .model-sub {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 9.5px;
    color: #64748b;
  }

  .meta-time {
    color: #a78bfa;
  }

  .card-right {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }

  .state-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 9px;
    font-weight: 700;
    padding: 2.5px 6px;
    border-radius: 4px;
  }

  .state-pill.active {
    background: rgba(34, 197, 94, 0.16);
    color: #4ade80;
    border: 1px solid rgba(34, 197, 94, 0.35);
  }

  .state-pill.done {
    background: rgba(16, 185, 129, 0.14);
    color: #34d399;
    border: 1px solid rgba(16, 185, 129, 0.3);
  }

  .state-pill.failed {
    background: rgba(239, 68, 68, 0.15);
    color: #f87171;
    border: 1px solid rgba(239, 68, 68, 0.35);
  }

  .state-pill.standby {
    background: rgba(148, 163, 184, 0.1);
    color: #94a3b8;
    border: 1px solid rgba(148, 163, 184, 0.2);
  }

  .pulse-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #22c55e;
    animation: pulseGlow 1.2s infinite;
  }

  .inspect-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 7px;
    border-radius: 5px;
    font-size: 9.5px;
    background: rgba(139, 92, 246, 0.15);
    border: 1px solid rgba(139, 92, 246, 0.35);
    color: #ddd6fe;
    cursor: pointer;
  }

  .inspect-btn:hover {
    background: rgba(139, 92, 246, 0.3);
    color: #fff;
  }

  /* Column 3: Prompt Step XYNode */
  .trace-xy-node {
    padding: 7px 11px;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 3px;
    background: #111622;
    border-color: rgba(139, 92, 246, 0.32);
  }

  .trace-xy-node:hover {
    border-color: rgba(139, 92, 246, 0.65);
    background: #161c2c;
  }

  .trace-xy-node.running {
    border-color: rgba(34, 197, 94, 0.55);
    background: rgba(34, 197, 94, 0.07);
  }

  .trace-xy-node.failed {
    border-color: rgba(239, 68, 68, 0.5);
    background: rgba(239, 68, 68, 0.06);
  }

  .trace-node-top {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 10.5px;
  }

  .trace-step-badge {
    color: #a78bfa;
    font-weight: 700;
  }

  .trace-stage-name {
    color: #f1f5f9;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }

  .trace-dur {
    color: #34d399;
    font-size: 9.5px;
  }

  .trace-node-sub {
    display: flex;
    align-items: center;
    justify-content: space-between;
    font-size: 9.5px;
    color: #94a3b8;
  }

  .trace-model-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 150px;
  }

  .trace-inspect-link {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    color: #c4b5fd;
  }

  /* Bottom Prompt Chain Strip */
  .prompt-chain-strip {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    background: #0e121b;
    border-top: 1px solid rgba(255, 255, 255, 0.06);
    overflow-x: auto;
  }

  .chain-title {
    font-size: 10.5px;
    color: #94a3b8;
    white-space: nowrap;
  }

  .chain-pills {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }

  .chain-pill {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 8px;
    border-radius: 5px;
    background: rgba(255, 255, 255, 0.04);
    border: 1px solid rgba(255, 255, 255, 0.1);
    color: #cbd5e1;
    font-size: 10.5px;
    cursor: pointer;
    transition: background 0.15s ease;
  }

  .chain-pill:hover {
    background: rgba(139, 92, 246, 0.18);
    border-color: rgba(139, 92, 246, 0.45);
  }

  .chain-pill.running {
    border-color: rgba(34, 197, 94, 0.5);
    color: #86efac;
  }

  .chain-pill.failed {
    border-color: rgba(239, 68, 68, 0.5);
    color: #fca5a5;
  }

  .chain-num {
    color: #a78bfa;
    font-weight: 700;
  }

  .chain-stage {
    font-weight: 600;
    color: #f1f5f9;
  }

  .chain-model {
    color: #94a3b8;
  }

  .chain-dur {
    color: #64748b;
  }

  @keyframes pulseGlow {
    0%,
    100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.45;
      transform: scale(0.92);
    }
  }
</style>
