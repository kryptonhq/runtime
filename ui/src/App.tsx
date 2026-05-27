import { Routes, Route, Navigate } from "react-router-dom";
import { useEffect } from "react";
import Sidebar from "./components/Sidebar";
import AgentsList from "./pages/AgentsList";
import AgentDetail from "./pages/AgentDetail";
import LLMDetail from "./pages/LLMDetail";
import LLMList from "./pages/LLMList";
import MCPList from "./pages/MCPList";
import Settings from "./pages/Settings";
import { initTheme } from "./theme";

export default function App() {
  useEffect(() => {
    initTheme();
  }, []);

  return (
    <div className="h-screen flex overflow-hidden">
      <Sidebar />
      <main className="flex-1 min-w-0 overflow-y-auto px-8 py-8">
        <div className="max-w-6xl mx-auto w-full">
          <Routes>
            <Route path="/" element={<Navigate to="/agents" replace />} />
            <Route path="/agents" element={<AgentsList />} />
            <Route
              path="/agents/:namespace/:name"
              element={<AgentDetail />}
            />
            <Route path="/llms" element={<LLMList />} />
            <Route path="/llms/:namespace/:name" element={<LLMDetail />} />
            <Route path="/mcp" element={<MCPList />} />
            <Route
              path="/mcp/:namespace/:name"
              element={<AgentDetail />}
            />
            <Route path="/settings" element={<Settings />} />
            <Route path="*" element={<Navigate to="/agents" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}
