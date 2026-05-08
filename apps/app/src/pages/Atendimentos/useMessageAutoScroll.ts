import { useCallback, useEffect, useRef, useState } from "react";
import { isNearBottom } from "./autoScroll";

export default function useMessageAutoScroll(messageCount: number) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [showNewMessagesHint, setShowNewMessagesHint] = useState(false);
  const previousCountRef = useRef(0);
  const userNearBottomRef = useRef(true);

  const scrollToLatest = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    container.scrollTop = container.scrollHeight;
    userNearBottomRef.current = true;
    setShowNewMessagesHint(false);
  }, []);

  const handleContainerScroll = useCallback(() => {
    const container = containerRef.current;
    if (!container) return;
    const nearBottom = isNearBottom(container.scrollTop, container.clientHeight, container.scrollHeight);
    userNearBottomRef.current = nearBottom;
    if (nearBottom) {
      setShowNewMessagesHint(false);
    }
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    const previousCount = previousCountRef.current;
    const hasNewMessages = messageCount > previousCount;
    previousCountRef.current = messageCount;

    if (!container || messageCount === 0) {
      setShowNewMessagesHint(false);
      return;
    }

    if (previousCount === 0 || userNearBottomRef.current) {
      scrollToLatest();
      return;
    }

    if (hasNewMessages) {
      setShowNewMessagesHint(true);
    }
  }, [messageCount, scrollToLatest]);

  return {
    containerRef,
    showNewMessagesHint,
    scrollToLatest,
    handleContainerScroll,
  };
}
