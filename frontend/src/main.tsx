import { SELF_TOASTER } from "@/components/self-update-toast";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { queryClient } from "@/lib/api";
import { syncTheme } from "@/lib/theme";
import { QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./index.css";

syncTheme();

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider delayDuration={300}>
          <App />
          {/* Below the title bar, clear of the window's buttons. */}
          <Toaster position="top-center" offset={{ top: 56 }} />
          {/* AppShelf's own update, at the bottom right as in Vibe Atlas. */}
          <Toaster id={SELF_TOASTER} position="bottom-right" />
        </TooltipProvider>
      </QueryClientProvider>
    </StrictMode>,
  );
}
