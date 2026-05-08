import type { ComponentType } from "react";

type EmbeddedComponent = ComponentType<{ embedded?: boolean }>;

export function createEmbeddedTab(Component: EmbeddedComponent) {
  return function EmbeddedTab() {
    return <Component embedded />;
  };
}
