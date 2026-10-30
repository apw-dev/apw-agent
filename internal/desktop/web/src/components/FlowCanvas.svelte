<script lang="ts">
  import { Code2, ZoomIn, ZoomOut, Maximize2, RotateCcw, MousePointerSquareDashed } from "lucide-svelte";
  import type { BoardDiagramCanvas, BoardDecisionItem } from "../lib/api";
  import type { HighlightedCanvasElement } from "../lib/board";
  import { t } from "../lib/i18n";

  let {
    diagram,
    decisions = [],
    diagramMode = "all",
    selectedItems = [],
    onToggleHighlight,
    onJumpToCode
  }: {
    diagram: BoardDiagramCanvas;
    decisions?: BoardDecisionItem[];
    diagramMode?: "all" | "sequence" | "er" | "trace" | "xyflow";
    selectedItems?: HighlightedCanvasElement[];
    onToggleHighlight: (el: HighlightedCanvasElement) => void;
    onJumpToCode: (file: string, line: number, label: string, preferDiff?: boolean) => void;
  } = $props();

  // Interactive XYFlow Node-Based Canvas state
  interface XYNode {
    id: string;
    kind: "sequence-node" | "er-table" | "trace-note";
    badge: string;
    title: string;
    subtitle: string;
    file: string;
    line: number;
    x: number;
    y: number;
    width: number;
    height: number;
    handleY: number;
    focal?: boolean;
    pinAlert?: boolean;
    columns?: Array<{ name: string; type: string; key?: string }>;
  }

  interface XYEdge {
    id: string;
    from: string;
    to: string;
    fromSide: "left" | "right";
    toSide: "left" | "right";
    curveOffset?: number;
    label: string;
    sublabel?: string;
    file: string;
    line: number;
    style: "request" | "return" | "primary" | "async";
  }

  let zoom = $state(0.88);
  let panX = $state(24);
  let panY = $state(18);
  let isPanning = $state(false);
  let panStart = { x: 0, y: 0, ox: 0, oy: 0 };

  let draggingNodeId = $state<string | null>(null);
  let dragStart = { x: 0, y: 0, nx: 0, ny: 0 };
  let nodePositions = $state<Record<string, { x: number; y: number }>>({});

  // Interactive Handle-to-Handle Connection Dragging
  let connectingFrom = $state<{
    nodeId: string;
    side: "left" | "right";
    sx: number;
    sy: number;
    cx: number;
    cy: number;
  } | null>(null);
  let userCustomEdges = $state<XYEdge[]>([]);

  // Rubber-band box selection for Visual Feedback Loop
  let lassoMode = $state(false);
  let lassoRect = $state<{ x1: number; y1: number; x2: number; y2: number } | null>(null);
  let canvasEl = $state<HTMLDivElement | null>(null);

  const actorTagMap = ["EXT", "EDGE", "ORIG", "ASY"];

  const defaultXYNodes = $derived.by((): XYNode[] => {
    const out: XYNode[] = [];

    // Row 1: Sequence / Call-Chain Nodes (pushed down from top border with wide horizontal gap so labels never cover cards)
    diagram.sequence_nodes.forEach((n, idx) => {
      const pos = nodePositions[n.id] || { x: 72 + idx * 305, y: 68 };
      out.push({
        id: n.id,
        kind: "sequence-node",
        badge: actorTagMap[idx % actorTagMap.length],
        title: n.label,
        subtitle: n.role,
        file: n.file,
        line: n.start_line,
        x: pos.x,
        y: pos.y,
        width: 210,
        height: 108,
        handleY: 54,
        focal: idx === 2
      });
    });

    // Row 2: ER / Data Schema Models
    diagram.er_tables.forEach((t, idx) => {
      const id = t.id;
      const pos = nodePositions[id] || { x: 72 + idx * 365, y: 248 };
      out.push({
        id,
        kind: "er-table",
        badge: "MODEL / TABLE",
        title: t.name,
        subtitle: `${t.file}:L${t.line}`,
        file: t.file,
        line: t.line,
        x: pos.x,
        y: pos.y,
        width: 250,
        height: 146,
        handleY: 73,
        columns: t.columns.slice(0, 4)
      });
    });

    // Row 3: Trace Notes & Red-Pin Decision Nodes
    diagram.trace_notes.forEach((tn, idx) => {
      const id = tn.id;
      const pos = nodePositions[id] || { x: 72 + idx * 365, y: 452 };
      out.push({
        id,
        kind: "trace-note",
        badge: tn.timestamp,
        title: tn.title,
        subtitle: tn.summary,
        file: tn.file,
        line: tn.line,
        x: pos.x,
        y: pos.y,
        width: 255,
        height: 112,
        handleY: 56,
        pinAlert: idx === 0 && decisions.some((d) => d.pinned_alert)
      });
    });

    return out;
  });

  const xyEdges = $derived.by((): XYEdge[] => {
    const edges: XYEdge[] = [];
    const styles: Array<"request" | "primary" | "return" | "async"> = [
      "request",
      "primary",
      "return",
      "async"
    ];

    // Keep primary pipeline edges + at most 1 cross-call edge so handle connections stay crisp
    const seqEdgesToRender = diagram.sequence_edges.slice(0, 4);
    seqEdgesToRender.forEach((e, i) => {
      edges.push({
        id: e.id,
        from: e.from,
        to: e.to,
        fromSide: "right",
        toSide: "left",
        curveOffset: i >= 3 ? -68 : -38,
        label: e.label,
        sublabel: e.symbol,
        file: e.file,
        line: e.line,
        style: styles[i % styles.length]
      });
    });

    // Connect ER Tables via Right Handle -> Left Handle
    if (diagram.er_tables.length >= 2) {
      edges.push({
        id: "xy-er-rel-1",
        from: diagram.er_tables[0].id,
        to: diagram.er_tables[1].id,
        fromSide: "right",
        toSide: "left",
        curveOffset: -34,
        label: `1 : N · ${diagram.er_tables[1].file.split("/").pop()}:L${diagram.er_tables[1].line}`,
        file: diagram.er_tables[1].file,
        line: diagram.er_tables[1].line,
        style: "return"
      });
    }
    if (diagram.er_tables.length >= 3) {
      edges.push({
        id: "xy-er-rel-2",
        from: diagram.er_tables[1].id,
        to: diagram.er_tables[2].id,
        fromSide: "right",
        toSide: "left",
        curveOffset: -34,
        label: `1 : N · ${diagram.er_tables[2].file.split("/").pop()}:L${diagram.er_tables[2].line}`,
        file: diagram.er_tables[2].file,
        line: diagram.er_tables[2].line,
        style: "async"
      });
    }

    // Connect Sequence Node #1 Left Handle -> Trace Note #01 Left Handle, and chain Trace Notes via Right -> Left Handles
    if (diagram.trace_notes.length > 0 && diagram.sequence_nodes.length > 0) {
      edges.push({
        id: "xy-trace-bind-1",
        from: diagram.sequence_nodes[0].id,
        to: diagram.trace_notes[0].id,
        fromSide: "left",
        toSide: "left",
        label: "Trace Binding",
        file: diagram.trace_notes[0].file,
        line: diagram.trace_notes[0].line,
        style: "async"
      });
    }
    if (diagram.trace_notes.length >= 2) {
      edges.push({
        id: "xy-trace-chain-1",
        from: diagram.trace_notes[0].id,
        to: diagram.trace_notes[1].id,
        fromSide: "right",
        toSide: "left",
        curveOffset: -34,
        label: "Trace Step #02",
        file: diagram.trace_notes[1].file,
        line: diagram.trace_notes[1].line,
        style: "return"
      });
    }
    if (diagram.trace_notes.length >= 3) {
      edges.push({
        id: "xy-trace-chain-2",
        from: diagram.trace_notes[1].id,
        to: diagram.trace_notes[2].id,
        fromSide: "right",
        toSide: "left",
        curveOffset: -34,
        label: "Decision Pin Scan",
        file: diagram.trace_notes[2].file,
        line: diagram.trace_notes[2].line,
        style: "primary"
      });
    }

    return [...edges, ...userCustomEdges];
  });

  function isSelected(id: string): boolean {
    return selectedItems.some((item) => item.id === id);
  }

  function resetLayout() {
    nodePositions = {};
    userCustomEdges = [];
    zoom = 0.88;
    panX = 24;
    panY = 18;
  }

  function getHandleCoords(node: XYNode, side: "left" | "right"): { x: number; y: number } {
    return {
      x: side === "left" ? node.x : node.x + node.width,
      y: node.y + node.handleY
    };
  }

  function onHandlePointerDown(e: PointerEvent, node: XYNode, side: "left" | "right") {
    e.stopPropagation();
    const pt = getHandleCoords(node, side);
    connectingFrom = {
      nodeId: node.id,
      side,
      sx: pt.x,
      sy: pt.y,
      cx: pt.x,
      cy: pt.y
    };
  }

  function onHandlePointerUp(e: PointerEvent, targetNode: XYNode, targetSide: "left" | "right") {
    if (!connectingFrom) return;
    e.stopPropagation();
    if (connectingFrom.nodeId !== targetNode.id) {
      const id = `custom-edge-${connectingFrom.nodeId}-${targetNode.id}-${Date.now()}`;
      userCustomEdges = [
        ...userCustomEdges,
        {
          id,
          from: connectingFrom.nodeId,
          to: targetNode.id,
          fromSide: connectingFrom.side,
          toSide: targetSide,
          label: `Link → ${targetNode.title.slice(0, 14)}`,
          file: targetNode.file,
          line: targetNode.line,
          style: "primary"
        }
      ];
    }
    connectingFrom = null;
  }

  function onNodePointerDown(e: PointerEvent, node: XYNode) {
    if ((e.target as HTMLElement)?.closest("button, .xy-handle")) return;
    e.stopPropagation();
    draggingNodeId = node.id;
    dragStart = { x: e.clientX, y: e.clientY, nx: node.x, ny: node.y };
  }

  function onCanvasPointerDown(e: PointerEvent) {
    if ((e.target as HTMLElement)?.closest(".xy-node, .xy-controls, .xy-edge-pill, .xy-handle")) return;
    if (lassoMode || e.shiftKey) {
      if (!canvasEl) return;
      const rect = canvasEl.getBoundingClientRect();
      const x = (e.clientX - rect.left - panX) / zoom;
      const y = (e.clientY - rect.top - panY) / zoom;
      lassoRect = { x1: x, y1: y, x2: x, y2: y };
      return;
    }
    isPanning = true;
    panStart = { x: e.clientX, y: e.clientY, ox: panX, oy: panY };
  }

  function onCanvasPointerMove(e: PointerEvent) {
    if (connectingFrom && canvasEl) {
      const rect = canvasEl.getBoundingClientRect();
      const x = (e.clientX - rect.left - panX) / zoom;
      const y = (e.clientY - rect.top - panY) / zoom;
      connectingFrom = { ...connectingFrom, cx: x, cy: y };
      return;
    }
    if (draggingNodeId) {
      const dx = (e.clientX - dragStart.x) / zoom;
      const dy = (e.clientY - dragStart.y) / zoom;
      nodePositions = {
        ...nodePositions,
        [draggingNodeId]: {
          x: Math.max(8, Math.round(dragStart.nx + dx)),
          y: Math.max(8, Math.round(dragStart.ny + dy))
        }
      };
      return;
    }
    if (lassoRect && canvasEl) {
      const rect = canvasEl.getBoundingClientRect();
      const x = (e.clientX - rect.left - panX) / zoom;
      const y = (e.clientY - rect.top - panY) / zoom;
      lassoRect = { ...lassoRect, x2: x, y2: y };
      return;
    }
    if (isPanning) {
      panX = panStart.ox + (e.clientX - panStart.x);
      panY = panStart.oy + (e.clientY - panStart.y);
    }
  }

  function onCanvasPointerUp() {
    if (connectingFrom) {
      // Check if cursor landed inside any other node and connect to its closest handle
      for (const n of defaultXYNodes) {
        if (
          n.id !== connectingFrom.nodeId &&
          connectingFrom.cx >= n.x - 14 &&
          connectingFrom.cx <= n.x + n.width + 14 &&
          connectingFrom.cy >= n.y - 10 &&
          connectingFrom.cy <= n.y + n.height + 10
        ) {
          const targetSide: "left" | "right" =
            connectingFrom.cx > n.x + n.width / 2 ? "right" : "left";
          const id = `custom-edge-${connectingFrom.nodeId}-${n.id}-${Date.now()}`;
          userCustomEdges = [
            ...userCustomEdges,
            {
              id,
              from: connectingFrom.nodeId,
              to: n.id,
              fromSide: connectingFrom.side,
              toSide: targetSide,
              label: `Link → ${n.title.slice(0, 14)}`,
              file: n.file,
              line: n.line,
              style: "primary"
            }
          ];
          break;
        }
      }
      connectingFrom = null;
    }
    if (lassoRect) {
      const minX = Math.min(lassoRect.x1, lassoRect.x2);
      const maxX = Math.max(lassoRect.x1, lassoRect.x2);
      const minY = Math.min(lassoRect.y1, lassoRect.y2);
      const maxY = Math.max(lassoRect.y1, lassoRect.y2);
      if (maxX - minX > 12 && maxY - minY > 12) {
        for (const n of defaultXYNodes) {
          const cx = n.x + n.width / 2;
          const cy = n.y + n.height / 2;
          if (cx >= minX && cx <= maxX && cy >= minY && cy <= maxY && !isSelected(n.id)) {
            onToggleHighlight({
              id: n.id,
              kind: n.kind,
              label: n.title,
              file: n.file,
              line: n.line,
              detail: n.subtitle
            });
          }
        }
      }
      lassoRect = null;
    }
    draggingNodeId = null;
    isPanning = false;
  }

  /**
   * Computes a cubic Bezier path that ALWAYS starts and ends at the exact center
   * of the orange Handle dots (`.xy-handle.left` / `.xy-handle.right`) on the nodes.
   */
  function computeBezierPath(
    fromNode: XYNode | undefined,
    toNode: XYNode | undefined,
    edge: XYEdge
  ): {
    d: string;
    sx: number;
    sy: number;
    tx: number;
    ty: number;
    mx: number;
    my: number;
  } {
    if (!fromNode || !toNode) {
      return { d: "", sx: 0, sy: 0, tx: 0, ty: 0, mx: 0, my: 0 };
    }
    const startPt = getHandleCoords(fromNode, edge.fromSide);
    const endPt = getHandleCoords(toNode, edge.toSide);
    const sx = startPt.x;
    const sy = startPt.y;
    const tx = endPt.x;
    const ty = endPt.y;

    const dirFrom = edge.fromSide === "left" ? -1 : 1;
    const dirTo = edge.toSide === "right" ? 1 : -1;
    const dx = Math.abs(tx - sx);
    const dy = Math.abs(ty - sy);
    const pull = Math.max(dx * 0.46, Math.min(130, Math.hypot(dx, dy) * 0.38), 58);
    const arch = edge.curveOffset || 0;

    const cpx1 = sx + dirFrom * pull;
    const cpy1 = sy + arch;
    const cpx2 = tx + dirTo * pull;
    const cpy2 = ty + arch;

    // Exact midpoint at t = 0.5 on cubic Bezier curve
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

  // Sequence Diagram Lifeline Coordinates (matching uploaded Reference Image 1)
  const seqActors = $derived(
    diagram.sequence_nodes.slice(0, 4).map((n, i) => ({
      ...n,
      tag: actorTagMap[i % actorTagMap.length],
      focal: i === 2,
      asyncActor: i === 3,
      cx: 130 + i * 220
    }))
  );
</script>

<!-- 1. AUTHENTIC VISUAL SEQUENCE LIFELINE DIAGRAM (Matching Reference Image 1) -->
{#if diagramMode === "all" || diagramMode === "sequence"}
  <div class="seq-diagram-frame" data-testid="visual-sequence-diagram">
    <div class="seq-diagram-top">
      <div>
        <strong>{$t("Sequence Lifeline Diagram (Code-to-Diagram Binding)", "Sequence Lifeline Diagram (ผูกตำแหน่งไฟล์และบรรทัดโค้ดจริง)")}</strong>
        <span class="seq-sub">{$t(
          "Click any Actor box, Activation bar, or arrow to jump to live code · Click '+ Highlight' to copy context",
          "คลิกที่กล่อง Actor, แท่ง Activation หรือเส้นลูกศรเพื่อเปิดโค้ดจริงด้านขวา · กด \"+ ไฮไลต์สั่งแก้\" เพื่อคัดลอกบริบท"
        )}</span>
      </div>
    </div>

    <div class="seq-svg-scroll">
      <svg
        viewBox="0 0 920 450"
        class="seq-svg"
        role="img"
        aria-label={$t(
          "Sequence Diagram showing system execution flow and links to live code",
          "Sequence Diagram แสดงลำดับการทำงานของระบบและลิงก์ไปยังโค้ดจริง"
        )}
      >
        <defs>
          <marker id="arrow-req" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" markerHeight="6.5" orient="auto-start-reverse">
            <path d="M 0 1.5 L 10 5 L 0 8.5 z" fill="#3b6398" />
          </marker>
          <marker id="arrow-ret" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" markerHeight="6.5" orient="auto-start-reverse">
            <path d="M 0 1.5 L 10 5 L 0 8.5 z" fill="#526071" />
          </marker>
          <marker id="arrow-pri" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6.5" markerHeight="6.5" orient="auto-start-reverse">
            <path d="M 0 1.5 L 10 5 L 0 8.5 z" fill="#b84a27" />
          </marker>
        </defs>

        <!-- Vertical Dashed Lifelines -->
        {#each seqActors as actor (actor.id)}
          <line
            x1={actor.cx}
            y1="88"
            x2={actor.cx}
            y2="395"
            class="lifeline"
          />
        {/each}

        <!-- Actor Header Boxes -->
        {#each seqActors as actor (actor.id)}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <g
            class="actor-group"
            class:selected={isSelected(actor.id)}
            onclick={() => onJumpToCode(actor.file, actor.start_line, actor.label)}
          >
            <rect
              x={actor.cx - 88}
              y="18"
              width="176"
              height="68"
              rx="7"
              class="actor-box"
              class:focal={actor.focal}
              class:async-box={actor.asyncActor}
            />
            <!-- Role Tag Pill (EXT / EDGE / ORIG / ASY) -->
            <rect
              x={actor.cx - 78}
              y="26"
              width="38"
              height="16"
              rx="3"
              class="actor-tag-bg"
              class:focal-tag={actor.focal}
            />
            <text x={actor.cx - 59} y="37" text-anchor="middle" class="actor-tag-txt">
              {actor.tag}
            </text>

            <text x={actor.cx} y="56" text-anchor="middle" class="actor-title">
              {actor.label.length > 22 ? actor.label.slice(0, 20) + "…" : actor.label}
            </text>
            <text x={actor.cx} y="73" text-anchor="middle" class="actor-sub">
              {actor.file.split("/").pop()}:L{actor.start_line}
            </text>
          </g>
        {/each}

        <!-- Activation Bars on Lifelines -->
        {#if seqActors.length >= 3}
          <!-- Activation on Actor 2 (EDGE / AST) -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <rect
            x={seqActors[1].cx - 5}
            y="128"
            width="10"
            height="198"
            rx="1.5"
            class="activation-bar"
            onclick={() => onJumpToCode(seqActors[1].file, seqActors[1].start_line, seqActors[1].label)}
          />
          <!-- Activation on Actor 3 (ORIG / Diff Engine) -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <rect
            x={seqActors[2].cx - 5}
            y="174"
            width="10"
            height="108"
            rx="1.5"
            class="activation-bar"
            onclick={() => onJumpToCode(seqActors[2].file, seqActors[2].start_line, seqActors[2].label)}
          />
        {/if}

        <!-- Step 1: Request Arrow (Actor 1 -> Actor 2) -->
        {#if seqActors.length >= 2}
          {@const edge1 = diagram.sequence_edges[0]}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <g
            class="seq-arrow-grp"
            onclick={() =>
              edge1 && onJumpToCode(edge1.file, edge1.line, edge1.symbol)}
          >
            <text
              x={(seqActors[0].cx + seqActors[1].cx) / 2}
              y="120"
              text-anchor="middle"
              class="msg-label req-txt"
            >
              {edge1 ? `${edge1.symbol.toUpperCase()} · ${edge1.file.split("/").pop()}:L${edge1.line}` : "GET /API/BOARD · INSPECT"}
            </text>
            <line
              x1={seqActors[0].cx}
              y1="128"
              x2={seqActors[1].cx - 6}
              y2="128"
              class="msg-line req-line"
              marker-end="url(#arrow-req)"
            />
          </g>
        {/if}

        <!-- Step 2: Origin Call Arrow (Actor 2 -> Actor 3) -->
        {#if seqActors.length >= 3}
          {@const edge2 = diagram.sequence_edges[1]}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <g
            class="seq-arrow-grp"
            onclick={() =>
              edge2 && onJumpToCode(edge2.file, edge2.line, edge2.symbol)}
          >
            <text
              x={(seqActors[1].cx + seqActors[2].cx) / 2}
              y="166"
              text-anchor="middle"
              class="msg-label ret-txt"
            >
              {edge2 ? `AST STRUCTURAL DIFF · ${edge2.symbol.toUpperCase()}` : "PARSE AST · STRUCTURAL DIFF"}
            </text>
            <line
              x1={seqActors[1].cx + 6}
              y1="174"
              x2={seqActors[2].cx - 6}
              y2="174"
              class="msg-line ret-solid-line"
              marker-end="url(#arrow-ret)"
            />

            <!-- Self-Call Loop on Actor 3 (RENDER PSEUDOCODE & AUTO-COLLAPSE) -->
            <polyline
              points={`${seqActors[2].cx + 6},208 ${seqActors[2].cx + 42},208 ${seqActors[2].cx + 42},238 ${seqActors[2].cx + 7},238`}
              fill="none"
              class="msg-line ret-solid-line"
              marker-end="url(#arrow-ret)"
            />
            <text
              x={seqActors[2].cx + 52}
              y="226"
              text-anchor="start"
              class="msg-label ret-txt"
            >
              PSEUDOCODE + COLLAPSE TESTS
            </text>

            <!-- Step 3: Dashed Return Arrow (Actor 3 -> Actor 2) -->
            <text
              x={(seqActors[1].cx + seqActors[2].cx) / 2}
              y="270"
              text-anchor="middle"
              class="msg-label ret-txt"
            >
              200 · PSEUDOCODE + RED-PIN LOG
            </text>
            <line
              x1={seqActors[2].cx - 6}
              y1="278"
              x2={seqActors[1].cx + 6}
              y2="278"
              class="msg-line ret-dash-line"
              marker-end="url(#arrow-ret)"
            />

            <!-- Step 4: Primary Response Arrow (Actor 2 -> Actor 1) -->
            <text
              x={(seqActors[0].cx + seqActors[1].cx) / 2}
              y="314"
              text-anchor="middle"
              class="msg-label pri-txt"
            >
              200 · CODE-TO-DIAGRAM BOUND
            </text>
            <line
              x1={seqActors[1].cx - 6}
              y1="322"
              x2={seqActors[0].cx + 2}
              y2="322"
              class="msg-line pri-line"
              marker-end="url(#arrow-pri)"
            />
          </g>
        {/if}

        <!-- Step 5: Full-Width Async Trace Beacon (Actor 1 -> Actor 4) -->
        {#if seqActors.length >= 4}
          {@const edge3 = diagram.sequence_edges[2]}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <g
            class="seq-arrow-grp"
            onclick={() =>
              edge3 && onJumpToCode(edge3.file, edge3.line, edge3.symbol)}
          >
            <text
              x={(seqActors[2].cx + seqActors[3].cx) / 2}
              y="362"
              text-anchor="middle"
              class="msg-label ret-txt"
            >
              READ-ONLY FILE BINDING · {edge3 ? `${edge3.file.split("/").pop()}:L${edge3.line}` : "TRACE SYNC"}
            </text>
            <line
              x1={seqActors[0].cx}
              y1="370"
              x2={seqActors[3].cx - 2}
              y2="370"
              class="msg-line ret-dash-line"
              marker-end="url(#arrow-ret)"
            />
          </g>
        {/if}
      </svg>
    </div>

    <!-- Interactive Quick-Jump & Highlight Strip + Legend (Matching Reference Image 1) -->
    <div class="seq-QuickStrip">
      {#each diagram.sequence_nodes as node, idx (node.id)}
        <div class="seq-pill-item" class:selected={isSelected(node.id)}>
          <button
            type="button"
            class="seq-jump-btn mono"
            onclick={() => onJumpToCode(node.file, node.start_line, node.label)}
            data-testid={`jump-${node.id}`}
          >
            <span class="tag-mini">[{actorTagMap[idx % actorTagMap.length]}]</span>
            <span>{node.label}</span>
            <span class="loc">{node.file.split("/").pop()}:L{node.start_line}</span>
          </button>
          <button
            type="button"
            class="seq-hl-btn"
            class:active={isSelected(node.id)}
            onclick={() =>
              onToggleHighlight({
                id: node.id,
                kind: "sequence-node",
                label: node.label,
                file: node.file,
                line: node.start_line,
                detail: node.description
              })}
            data-testid={`highlight-${node.id}`}
          >
            {isSelected(node.id)
              ? $t("✓ Highlighted", "✓ ไฮไลต์แล้ว")
              : $t("+ Highlight", "+ ไฮไลต์สั่งแก้")}
          </button>
        </div>
      {/each}
    </div>

    <div class="seq-legend" aria-label={$t("Sequence Diagram Legend", "คำอธิบายสัญลักษณ์ Sequence Diagram")}>
      <span class="legend-title mono">LEGEND</span>
      <div class="legend-items">
        <span class="leg-item">
          <span class="leg-box focal"></span>
          <span>{$t("Focal actor (Core engine)", "Focal actor (แกนประมวลผลหลัก)")}</span>
        </span>
        <span class="leg-item">
          <span class="leg-act"></span>
          <span>{$t("Activation (Active span)", "Activation (ช่วงกำลังทำงาน)")}</span>
        </span>
        <span class="leg-item">
          <span class="leg-line req"></span>
          <span>Request / AST Call</span>
        </span>
        <span class="leg-item">
          <span class="leg-line ret"></span>
          <span>Return / async trace</span>
        </span>
        <span class="leg-item">
          <span class="leg-line pri"></span>
          <span>Primary response (Code-bound)</span>
        </span>
      </div>
    </div>
  </div>
{/if}

<!-- 2. XYFLOW / SVELTE FLOW INTERACTIVE NODE-BASED CANVAS -->
{#if diagramMode === "all" || diagramMode === "xyflow" || diagramMode === "er" || diagramMode === "trace"}
  <div class="xyflow-shell" data-testid="xyflow-canvas-shell">
    <div class="xyflow-topbar">
      <div class="xyflow-title">
        <strong>XYFlow / Svelte Flow Interactive Node Canvas</strong>
        <span class="xy-hint">{$t(
          "Drag nodes to arrange · Click nodes/edges to jump to code · Toggle 'Box Select' (or hold Shift + drag) to select multiple nodes into Clipboard",
          "ลากโหนดเพื่อจัดวาง · คลิกโหนด/เส้นเชื่อมเพื่อเปิดโค้ดจริง · กดปุ่ม \"โหมดลากคลุมสั่งแก้\" (หรือกด Shift ค้างแล้วลากเมาส์) เพื่อเลือกหลายโหนดเข้า Clipboard"
        )}</span>
      </div>
      <div class="xyflow-actions">
        <button
          type="button"
          class="xy-mode-btn"
          class:active={lassoMode}
          onclick={() => (lassoMode = !lassoMode)}
          data-testid="xyflow-lasso-btn"
        >
          <MousePointerSquareDashed size={13} />
          <span>{lassoMode
            ? $t("Box Select Active", "กำลังใช้โหมดลากคลุมสั่งแก้")
            : $t("Box Select Mode", "โหมดลากคลุมสั่งแก้ (Box Select)")}</span>
        </button>
      </div>
    </div>

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="xyflow-viewport"
      class:lasso-cursor={lassoMode}
      bind:this={canvasEl}
      onpointerdown={onCanvasPointerDown}
      onpointermove={onCanvasPointerMove}
      onpointerup={onCanvasPointerUp}
      data-testid="xyflow-viewport"
    >
      <div
        class="xyflow-plane"
        style={`transform: translate(${panX}px, ${panY}px) scale(${zoom});`}
      >
        <!-- SVG Bezier Connection Edges Layer -->
        <svg class="xyflow-edges-svg" width="1100" height="560">
          <defs>
            <marker id="xy-arr-req" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 1.5 L 10 5 L 0 8.5 z" fill="#3b6398" />
            </marker>
            <marker id="xy-arr-pri" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 1.5 L 10 5 L 0 8.5 z" fill="#b84a27" />
            </marker>
            <marker id="xy-arr-ret" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 1.5 L 10 5 L 0 8.5 z" fill="#66635c" />
            </marker>
          </defs>

          {#each xyEdges as edge (edge.id)}
            {@const fromNode = defaultXYNodes.find((n) => n.id === edge.from)}
            {@const toNode = defaultXYNodes.find((n) => n.id === edge.to)}
            {@const path = computeBezierPath(fromNode, toNode, edge)}
            {#if path.d}
              <!-- svelte-ignore a11y_click_events_have_key_events -->
              <!-- svelte-ignore a11y_no_static_element_interactions -->
              <g
                class="xy-edge-group"
                onclick={() => onJumpToCode(edge.file, edge.line, edge.label)}
              >
                <path
                  d={path.d}
                  class={`xy-edge-path ${edge.style}`}
                  marker-end={edge.style === "primary"
                    ? "url(#xy-arr-pri)"
                    : edge.style === "request"
                      ? "url(#xy-arr-req)"
                      : "url(#xy-arr-ret)"}
                />
                <!-- Exact Handle Anchor Dots at Start & End of Wire -->
                <circle cx={path.sx} cy={path.sy} r="3.5" class="xy-wire-socket" />
                <circle cx={path.tx} cy={path.ty} r="3.5" class="xy-wire-socket" />
                <g transform={`translate(${path.mx}, ${path.my})`}>
                  <rect x="-72" y="-10" width="144" height="19" rx="4" class="xy-edge-label-bg" />
                  <text x="0" y="3" text-anchor="middle" class="xy-edge-label-txt">
                    {edge.label.length > 24 ? edge.label.slice(0, 22) + "…" : edge.label}
                  </text>
                </g>
              </g>
            {/if}
          {/each}

          <!-- Live Connection Preview Wire while dragging from a Handle dot -->
          {#if connectingFrom}
            {@const dir = connectingFrom.side === "left" ? -1 : 1}
            {@const cpx1 = connectingFrom.sx + dir * 70}
            {@const cpx2 = connectingFrom.cx - dir * 70}
            <path
              d={`M ${connectingFrom.sx} ${connectingFrom.sy} C ${cpx1} ${connectingFrom.sy}, ${cpx2} ${connectingFrom.cy}, ${connectingFrom.cx} ${connectingFrom.cy}`}
              class="xy-edge-path primary live-connect-wire"
              marker-end="url(#xy-arr-pri)"
            />
          {/if}

          <!-- Rubber-band Selection Box -->
          {#if lassoRect}
            <rect
              x={Math.min(lassoRect.x1, lassoRect.x2)}
              y={Math.min(lassoRect.y1, lassoRect.y2)}
              width={Math.abs(lassoRect.x2 - lassoRect.x1)}
              height={Math.abs(lassoRect.y2 - lassoRect.y1)}
              class="lasso-box"
            />
          {/if}
        </svg>

        <!-- Interactive Draggable XYFlow Nodes -->
        {#each defaultXYNodes as node (node.id)}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="xy-node"
            class:focal={node.focal}
            class:pin-alert={node.pinAlert}
            class:selected={isSelected(node.id)}
            style={`left:${node.x}px; top:${node.y}px; width:${node.width}px;`}
            onpointerdown={(e) => onNodePointerDown(e, node)}
            data-testid={`xynode-${node.id}`}
          >
            <!-- Left Handle Dot (Target / Source) -->
            <span
              class="xy-handle left"
              style={`top:${node.handleY - 1.5}px;`}
              title={$t("Left Handle · Drag to connect a new wire", "จุดเชื่อมต่อฝั่งซ้าย (Left Handle) · ลากเพื่อเชื่อมเส้นใหม่")}
              onpointerdown={(e) => onHandlePointerDown(e, node, "left")}
              onpointerup={(e) => onHandlePointerUp(e, node, "left")}
            ></span>

            <div class="xy-node-hdr">
              <span class="xy-badge mono" class:focal={node.focal} class:pin={node.pinAlert}>
                {node.badge}
              </span>
              <button
                type="button"
                class="xy-hl-chip"
                class:active={isSelected(node.id)}
                onclick={() =>
                  onToggleHighlight({
                    id: node.id,
                    kind: node.kind,
                    label: node.title,
                    file: node.file,
                    line: node.line,
                    detail: node.subtitle
                  })}
              >
                {isSelected(node.id)
                  ? $t("✓ Highlight", "✓ ไฮไลต์")
                  : $t("+ Highlight", "+ ไฮไลต์")}
              </button>
            </div>

            <div class="xy-node-title">{node.title}</div>
            <div class="xy-node-sub">{node.subtitle}</div>

            {#if node.columns && node.columns.length > 0}
              <div class="xy-er-cols mono">
                {#each node.columns as col (col.name)}
                  <div class="xy-er-row">
                    <span>{col.name}</span>
                    <span class="xy-col-type">{col.key || col.type}</span>
                  </div>
                {/each}
              </div>
            {/if}

            <button
              type="button"
              class="xy-code-btn mono"
              onclick={() => onJumpToCode(node.file, node.line, node.title)}
              title={`${node.file}:L${node.line}`}
            >
              <Code2 size={11} />
              <span class="xy-file-loc">{node.file}:L{node.line}</span>
              <span class="xy-go">{$t("Open Code →", "เปิดโค้ด →")}</span>
            </button>

            <!-- Right Handle Dot (Source / Target) -->
            <span
              class="xy-handle right"
              style={`top:${node.handleY - 1.5}px;`}
              title={$t("Right Handle · Drag to connect a new wire", "จุดเชื่อมต่อฝั่งขวา (Right Handle) · ลากเพื่อเชื่อมเส้นใหม่")}
              onpointerdown={(e) => onHandlePointerDown(e, node, "right")}
              onpointerup={(e) => onHandlePointerUp(e, node, "right")}
            ></span>
          </div>
        {/each}
      </div>

      <!-- XYFlow Controls (Bottom-Left) -->
      <div class="xy-controls" role="group" aria-label={$t("XYFlow zoom and layout controls", "ควบคุมการซูมและจัดวางกระดาน XYFlow")}>
        <button
          type="button"
          class="xy-ctrl-btn"
          onclick={() => (zoom = Math.min(1.5, Number((zoom + 0.12).toFixed(2))))}
          title={$t("Zoom In", "ซูมเข้า (Zoom In)")}
          aria-label={$t("Zoom In", "ซูมเข้า")}
        >
          <ZoomIn size={13} />
        </button>
        <button
          type="button"
          class="xy-ctrl-btn"
          onclick={() => (zoom = Math.max(0.6, Number((zoom - 0.12).toFixed(2))))}
          title={$t("Zoom Out", "ซูมออก (Zoom Out)")}
          aria-label={$t("Zoom Out", "ซูมออก")}
        >
          <ZoomOut size={13} />
        </button>
        <button
          type="button"
          class="xy-ctrl-btn"
          onclick={() => {
            zoom = 0.85;
            panX = 20;
            panY = 16;
          }}
          title={$t("Fit View", "พอดีหน้าจอ (Fit View)")}
          aria-label={$t("Fit View", "พอดีหน้าจอ")}
        >
          <Maximize2 size={13} />
        </button>
        <button
          type="button"
          class="xy-ctrl-btn"
          onclick={resetLayout}
          title={$t("Reset Layout", "รีเซ็ตตำแหน่งโหนด (Reset Layout)")}
          aria-label={$t("Reset Layout", "รีเซ็ตตำแหน่งโหนด")}
        >
          <RotateCcw size={13} />
        </button>
        <span class="xy-zoom-readout mono">{Math.round(zoom * 100)}%</span>
      </div>

      <!-- XYFlow MiniMap (Bottom-Right) -->
      <div class="xy-minimap" aria-hidden="true">
        <span class="minimap-label mono">XYFLOW MINIMAP</span>
        <svg viewBox="0 0 1050 540" class="minimap-svg">
          {#each defaultXYNodes as n (n.id)}
            <rect
              x={n.x}
              y={n.y}
              width={n.width}
              height={n.height}
              rx="14"
              fill={isSelected(n.id) || n.focal ? "#b84a27" : "#8c887f"}
              opacity="0.75"
            />
          {/each}
        </svg>
      </div>
    </div>
  </div>
{/if}

<style>
  /* 1. Visual Sequence Lifeline Diagram Styles (Reference Image 1) */
  .seq-diagram-frame {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg);
    padding: 14px 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .seq-diagram-top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    font-size: 13px;
  }
  .seq-sub {
    display: block;
    font-size: 11.5px;
    color: var(--muted);
    margin-top: 2px;
  }
  .seq-svg-scroll {
    width: 100%;
    overflow-x: auto;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 8px;
    padding: 8px 4px;
  }
  .seq-svg {
    width: 100%;
    min-width: 780px;
    height: auto;
    display: block;
    font-family: "Noto Sans", "Noto Sans Thai", "Segoe UI", sans-serif;
  }
  .lifeline {
    stroke: var(--line-strong);
    stroke-width: 1.5;
    stroke-dasharray: 5 5;
  }
  .actor-group {
    cursor: pointer;
  }
  .actor-box {
    fill: var(--panel-subtle);
    stroke: var(--line-strong);
    stroke-width: 1.5;
    transition: stroke 120ms ease, fill 120ms ease;
  }
  .actor-group:hover .actor-box,
  .actor-group.selected .actor-box {
    stroke: var(--accent);
    stroke-width: 2;
  }
  .actor-box.focal {
    fill: var(--accent-soft);
    stroke: var(--accent);
    stroke-width: 1.75;
  }
  .actor-box.async-box {
    stroke-dasharray: 5 4;
  }
  .actor-tag-bg {
    fill: var(--panel);
    stroke: var(--line-strong);
    stroke-width: 1;
  }
  .actor-tag-bg.focal-tag {
    fill: var(--panel);
    stroke: var(--accent);
  }
  .actor-tag-txt {
    font-family: "Cascadia Code", Consolas, monospace;
    font-size: 9.5px;
    font-weight: 700;
    fill: var(--muted);
    letter-spacing: 0.04em;
  }
  .actor-title {
    font-size: 12.5px;
    font-weight: 700;
    fill: var(--text);
  }
  .actor-sub {
    font-family: "Cascadia Code", Consolas, monospace;
    font-size: 10.5px;
    fill: var(--muted);
  }
  .activation-bar {
    fill: var(--panel-subtle);
    stroke: var(--muted);
    stroke-width: 1.3;
    cursor: pointer;
  }
  .activation-bar:hover {
    stroke: var(--accent);
    fill: var(--accent-soft);
  }
  .seq-arrow-grp {
    cursor: pointer;
  }
  .seq-arrow-grp:hover .msg-label {
    font-weight: 700;
  }
  .msg-label {
    font-family: "Cascadia Code", Consolas, monospace;
    font-size: 10px;
    letter-spacing: 0.05em;
  }
  .req-txt {
    fill: #3b6398;
  }
  .ret-txt {
    fill: var(--muted);
  }
  .pri-txt {
    fill: var(--accent);
    font-weight: 600;
  }
  .msg-line {
    stroke-width: 1.75;
  }
  .req-line {
    stroke: #3b6398;
  }
  .ret-solid-line {
    stroke: #526071;
  }
  .ret-dash-line {
    stroke: #526071;
    stroke-dasharray: 6 4;
  }
  .pri-line {
    stroke: var(--accent);
    stroke-width: 2;
  }

  .seq-QuickStrip {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(205px, 1fr));
    gap: 8px;
  }
  .seq-pill-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    padding: 6px 8px;
    border-radius: 7px;
    border: 1px solid var(--line);
    background: var(--panel);
  }
  .seq-pill-item.selected {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .seq-jump-btn {
    border: none;
    background: transparent;
    color: var(--text);
    font-size: 11px;
    display: flex;
    align-items: center;
    gap: 5px;
    min-width: 0;
    flex: 1;
    text-align: left;
    padding: 0;
  }
  .seq-jump-btn:hover {
    color: var(--accent);
  }
  .tag-mini {
    color: var(--accent);
    font-weight: 700;
  }
  .seq-jump-btn .loc {
    color: var(--muted);
    margin-left: auto;
    font-size: 10px;
  }
  .seq-hl-btn {
    padding: 2px 7px;
    border-radius: 5px;
    border: 1px solid var(--line-strong);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 10.5px;
    white-space: nowrap;
  }
  .seq-hl-btn.active {
    background: var(--accent);
    color: var(--accent-fg);
    border-color: var(--accent);
  }

  .seq-legend {
    border-top: 1px solid var(--line);
    padding-top: 10px;
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
    font-size: 11.5px;
    color: var(--muted);
  }
  .legend-title {
    font-size: 10.5px;
    font-weight: 700;
    letter-spacing: 0.08em;
    color: var(--faint);
  }
  .legend-items {
    display: flex;
    align-items: center;
    gap: 18px;
    flex-wrap: wrap;
  }
  .leg-item {
    display: inline-flex;
    align-items: center;
    gap: 7px;
  }
  .leg-box.focal {
    width: 14px;
    height: 10px;
    border-radius: 2px;
    border: 1.5px solid var(--accent);
    background: var(--accent-soft);
  }
  .leg-act {
    width: 6px;
    height: 14px;
    border-radius: 1px;
    border: 1.2px solid var(--muted);
    background: var(--panel-subtle);
  }
  .leg-line {
    width: 24px;
    height: 0;
    display: inline-block;
  }
  .leg-line.req {
    border-top: 2px solid #3b6398;
  }
  .leg-line.ret {
    border-top: 2px dashed #526071;
  }
  .leg-line.pri {
    border-top: 2px solid var(--accent);
  }

  /* 2. XYFlow / Svelte Flow Interactive Node Canvas Styles */
  .xyflow-shell {
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg);
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .xyflow-topbar {
    padding: 10px 14px;
    border-bottom: 1px solid var(--line);
    background: var(--panel);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
    font-size: 12.5px;
  }
  .xy-hint {
    display: block;
    font-size: 11px;
    color: var(--muted);
    margin-top: 2px;
  }
  .xy-mode-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 5px 10px;
    border-radius: 6px;
    border: 1px solid var(--line-strong);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 11.5px;
    font-weight: 500;
  }
  .xy-mode-btn.active {
    background: var(--accent);
    color: var(--accent-fg);
    border-color: var(--accent);
  }
  .xyflow-viewport {
    position: relative;
    width: 100%;
    height: 580px;
    overflow: hidden;
    background-color: var(--bg);
    background-image: radial-gradient(var(--line-strong) 1px, transparent 1px);
    background-size: 18px 18px;
    cursor: grab;
    touch-action: none;
  }
  .xyflow-viewport:active {
    cursor: grabbing;
  }
  .xyflow-viewport.lasso-cursor {
    cursor: crosshair;
  }
  .xyflow-plane {
    position: absolute;
    left: 0;
    top: 0;
    width: 1320px;
    height: 620px;
    transform-origin: 0 0;
  }
  .xyflow-edges-svg {
    position: absolute;
    left: 0;
    top: 0;
    overflow: visible;
    pointer-events: none;
    z-index: 3;
  }
  .xy-edge-group {
    cursor: pointer;
    pointer-events: auto;
  }
  .xy-edge-path {
    fill: none;
    stroke-width: 1.8;
  }
  .xy-edge-path.request {
    stroke: #3b6398;
  }
  .xy-edge-path.primary {
    stroke: var(--accent);
    stroke-width: 2.2;
  }
  .xy-edge-path.return {
    stroke: var(--muted);
    stroke-dasharray: 6 4;
  }
  .xy-edge-path.async {
    stroke: var(--faint);
    stroke-dasharray: 4 4;
  }
  .live-connect-wire {
    stroke-dasharray: 5 3;
    pointer-events: none;
  }
  .xy-wire-socket {
    fill: var(--panel);
    stroke: var(--accent);
    stroke-width: 2;
  }
  .xy-edge-label-bg {
    fill: var(--panel);
    stroke: var(--line-strong);
    stroke-width: 1;
  }
  .xy-edge-label-txt {
    font-family: "Cascadia Code", Consolas, monospace;
    font-size: 9.5px;
    fill: var(--text);
  }
  .lasso-box {
    fill: var(--accent-soft);
    stroke: var(--accent);
    stroke-width: 1.5;
    stroke-dasharray: 4 3;
  }

  .xy-node {
    box-sizing: border-box;
    position: absolute;
    border-radius: 9px;
    border: 1.5px solid var(--line-strong);
    background: var(--panel);
    padding: 9px 11px;
    box-shadow: var(--shadow-sm);
    display: flex;
    flex-direction: column;
    gap: 5px;
    cursor: grab;
    user-select: none;
    z-index: 2;
  }
  .xy-node:active {
    cursor: grabbing;
  }
  .xy-node.focal {
    border-color: var(--accent);
    background: var(--panel);
  }
  .xy-node.pin-alert {
    border-color: var(--err);
  }
  .xy-node.selected {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }
  .xy-handle {
    box-sizing: border-box;
    position: absolute;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--panel);
    border: 2.2px solid var(--accent);
    transform: translate(-50%, -50%);
    cursor: crosshair;
    z-index: 5;
    transition: transform 120ms ease, box-shadow 120ms ease, background 120ms ease;
  }
  .xy-handle:hover {
    transform: translate(-50%, -50%) scale(1.3);
    background: var(--accent);
    box-shadow: 0 0 0 4px var(--accent-soft);
  }
  .xy-handle.left {
    left: -1.5px;
  }
  .xy-handle.right {
    left: calc(100% + 1.5px);
  }
  .xy-node-hdr {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
  }
  .xy-badge {
    font-size: 9.5px;
    font-weight: 700;
    padding: 1px 6px;
    border-radius: 4px;
    background: var(--panel-subtle);
    border: 1px solid var(--line);
    color: var(--muted);
  }
  .xy-badge.focal {
    color: var(--accent);
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .xy-badge.pin {
    color: var(--err);
    border-color: var(--err);
    background: var(--err-soft);
  }
  .xy-hl-chip {
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 4px;
    border: 1px solid var(--line);
    background: var(--bg);
    color: var(--muted);
  }
  .xy-hl-chip.active {
    background: var(--accent);
    color: var(--accent-fg);
    border-color: var(--accent);
  }
  .xy-node-title {
    font-size: 12px;
    font-weight: 700;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .xy-node-sub {
    font-size: 10.5px;
    color: var(--muted);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    line-height: 1.35;
  }
  .xy-er-cols {
    border-top: 1px solid var(--line);
    padding-top: 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 10px;
  }
  .xy-er-row {
    display: flex;
    justify-content: space-between;
  }
  .xy-col-type {
    color: var(--muted);
  }
  .xy-code-btn {
    margin-top: 2px;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px 7px;
    border-radius: 5px;
    border: 1px solid var(--line);
    background: var(--panel-subtle);
    color: var(--text);
    font-size: 10px;
  }
  .xy-code-btn:hover {
    border-color: var(--accent);
    color: var(--accent);
  }
  .xy-file-loc {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 145px;
  }
  .xy-go {
    margin-left: auto;
    color: var(--accent);
    font-weight: 600;
    flex-shrink: 0;
  }

  .xy-controls {
    position: absolute;
    left: 12px;
    bottom: 12px;
    display: flex;
    align-items: center;
    gap: 3px;
    padding: 4px;
    border-radius: 7px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    box-shadow: var(--shadow-sm);
  }
  .xy-ctrl-btn {
    width: 28px;
    height: 28px;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text);
    display: grid;
    place-items: center;
  }
  .xy-ctrl-btn:hover {
    background: var(--hover);
  }
  .xy-zoom-readout {
    font-size: 10.5px;
    padding: 0 6px;
    color: var(--muted);
  }
  .xy-minimap {
    position: absolute;
    right: 12px;
    bottom: 12px;
    width: 145px;
    height: 86px;
    border-radius: 7px;
    border: 1px solid var(--line-strong);
    background: var(--panel);
    padding: 4px 6px;
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow-sm);
  }
  .minimap-label {
    font-size: 8.5px;
    color: var(--faint);
    font-weight: 700;
  }
  .minimap-svg {
    width: 100%;
    flex: 1;
  }
</style>
