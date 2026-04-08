import { useCallback, useRef, useState } from 'react';
import { Pane, PaneSplit, useStore } from '../store';
import Terminal from './Terminal';
import MonacoEditor from './MonacoEditor';

interface SplitPaneProps {
  pane: Pane;
  visible: boolean;
}

export default function SplitPane({ pane, visible }: SplitPaneProps) {
  const { focusedPaneId, setFocusedPane, resizePanes } = useStore();

  if (pane.type === 'terminal') {
    const isFocused = pane.id === focusedPaneId;
    return (
      <div
        className={`pane-leaf ${isFocused ? 'pane-focused' : ''}`}
        onClick={() => setFocusedPane(pane.id)}
      >
        <Terminal terminalId={pane.terminalId!} visible={visible} focused={isFocused} />
      </div>
    );
  }

  if (pane.type === 'editor') {
    const isFocused = pane.id === focusedPaneId;
    return (
      <div
        className={`pane-leaf ${isFocused ? 'pane-focused' : ''}`}
        onClick={() => setFocusedPane(pane.id)}
      >
        <MonacoEditor filePath={pane.filePath!} language={pane.language || 'plaintext'} visible={visible} line={pane.line} />
      </div>
    );
  }

  // Split container
  return (
    <SplitContainer
      splitPane={pane as PaneSplit}
      visible={visible}
      onResize={(sizes) => resizePanes(pane.id, sizes)}
    />
  );
}

interface SplitContainerProps {
  splitPane: Extract<Pane, { type: 'horizontal' | 'vertical' }>;
  visible: boolean;
  onResize: (sizes: number[]) => void;
}

function SplitContainer({ splitPane, visible, onResize }: SplitContainerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [dragging, setDragging] = useState<number | null>(null);

  const isVertical = splitPane.type === 'vertical';

  const handleMouseDown = useCallback((dividerIndex: number) => {
    setDragging(dividerIndex);

    const handleMouseMove = (e: MouseEvent) => {
      if (!containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      const totalSize = isVertical ? rect.width : rect.height;
      const pos = isVertical ? e.clientX - rect.left : e.clientY - rect.top;
      const percent = (pos / totalSize) * 100;

      // Calculate new sizes
      const newSizes = [...splitPane.sizes];
      const minSize = 15; // minimum 15% per pane

      // For two panes
      if (newSizes.length === 2) {
        const clamped = Math.max(minSize, Math.min(100 - minSize, percent));
        newSizes[0] = clamped;
        newSizes[1] = 100 - clamped;
      } else {
        // For multi-pane: adjust the panes around the divider
        const before = percent;
        const after = 100 - percent;
        if (before >= minSize && after >= minSize) {
          // Distribute proportionally
          let sumBefore = 0;
          for (let i = 0; i <= dividerIndex; i++) sumBefore += newSizes[i];
          let sumAfter = 0;
          for (let i = dividerIndex + 1; i < newSizes.length; i++) sumAfter += newSizes[i];

          const scaleBefore = before / sumBefore;
          const scaleAfter = after / sumAfter;

          for (let i = 0; i <= dividerIndex; i++) newSizes[i] *= scaleBefore;
          for (let i = dividerIndex + 1; i < newSizes.length; i++) newSizes[i] *= scaleAfter;
        }
      }

      onResize(newSizes);
    };

    const handleMouseUp = () => {
      setDragging(null);
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    document.body.style.cursor = isVertical ? 'col-resize' : 'row-resize';
    document.body.style.userSelect = 'none';
  }, [isVertical, splitPane.sizes, onResize]);

  return (
    <div
      ref={containerRef}
      className={`split-container ${isVertical ? 'split-vertical' : 'split-horizontal'}`}
      style={{
        flexDirection: isVertical ? 'row' : 'column',
      }}
    >
      {splitPane.children.map((child, i) => (
        <div key={child.id} style={{ display: 'contents' }}>
          <div
            className="split-child"
            style={{
              [isVertical ? 'width' : 'height']: `calc(${splitPane.sizes[i]}% - ${i < splitPane.children.length - 1 ? 2 : 0}px)`,
              [isVertical ? 'height' : 'width']: '100%',
            }}
          >
            <SplitPane pane={child} visible={visible} />
          </div>
          {i < splitPane.children.length - 1 && (
            <div
              className={`split-divider ${isVertical ? 'divider-vertical' : 'divider-horizontal'} ${dragging === i ? 'divider-active' : ''}`}
              onMouseDown={() => handleMouseDown(i)}
            />
          )}
        </div>
      ))}
    </div>
  );
}
