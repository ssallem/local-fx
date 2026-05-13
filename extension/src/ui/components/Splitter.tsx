import { useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent, KeyboardEvent as ReactKeyboardEvent } from "react";

/**
 * Splitter — vertical/horizontal 공용 드래그 핸들. Pointer Capture 기반.
 * props: orientation, value(controlled px), onChange, min/max(px), ariaLabel, defaultValue(dblclick 복귀).
 */

export interface SplitterProps {
  orientation: "vertical" | "horizontal";
  value: number;
  onChange: (next: number) => void;
  min: number;
  max: number;
  ariaLabel: string;
  defaultValue?: number;
}

interface DragState {
  startCoord: number;
  startValue: number;
}

const KEY_STEP = 10;

export function Splitter({
  orientation,
  value,
  onChange,
  min,
  max,
  ariaLabel,
  defaultValue
}: SplitterProps): JSX.Element {
  const [dragging, setDragging] = useState<boolean>(false);
  const dragStateRef = useRef<DragState | null>(null);

  const clamp = (v: number, lo: number, hi: number): number =>
    Math.max(lo, Math.min(hi, v));

  function handlePointerDown(e: ReactPointerEvent<HTMLDivElement>): void {
    // Pointer Capture: 핸들 6px hit-area 밖으로 커서가 나가도 move 이벤트가
    // 계속 이 엘리먼트로 라우팅되도록. window listener 보다 cleanup이 깔끔.
    e.preventDefault();
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {
      // 일부 브라우저/환경에서 capture 실패 가능 — 캡처 없이도 드래그 자체는 동작.
    }
    dragStateRef.current = {
      startCoord: orientation === "vertical" ? e.clientX : e.clientY,
      startValue: value
    };
    setDragging(true);
  }

  function handlePointerMove(e: ReactPointerEvent<HTMLDivElement>): void {
    const state = dragStateRef.current;
    if (!state) return;
    if (!e.currentTarget.hasPointerCapture(e.pointerId)) return;
    const current = orientation === "vertical" ? e.clientX : e.clientY;
    const delta = current - state.startCoord;
    const next = clamp(state.startValue + delta, min, max);
    onChange(next);
  }

  function endDrag(e: ReactPointerEvent<HTMLDivElement>): void {
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
    dragStateRef.current = null;
    setDragging(false);
  }

  function handleKeyDown(e: ReactKeyboardEvent<HTMLDivElement>): void {
    let next: number | null = null;
    if (orientation === "vertical") {
      if (e.key === "ArrowLeft") next = value - KEY_STEP;
      else if (e.key === "ArrowRight") next = value + KEY_STEP;
    } else {
      if (e.key === "ArrowUp") next = value - KEY_STEP;
      else if (e.key === "ArrowDown") next = value + KEY_STEP;
    }
    if (e.key === "Home") next = min;
    else if (e.key === "End") next = max;

    if (next === null) return;
    e.preventDefault();
    onChange(clamp(next, min, max));
  }

  function handleDoubleClick(): void {
    if (defaultValue === undefined) return;
    onChange(clamp(defaultValue, min, max));
  }

  const className = `splitter splitter--${orientation}${dragging ? " splitter--dragging" : ""}`;

  return (
    <div
      className={className}
      role="separator"
      aria-orientation={orientation}
      aria-valuenow={value}
      aria-valuemin={min}
      aria-valuemax={max}
      aria-label={ariaLabel}
      tabIndex={0}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onKeyDown={handleKeyDown}
      onDoubleClick={handleDoubleClick}
    />
  );
}
