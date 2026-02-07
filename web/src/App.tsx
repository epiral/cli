import { Routes, Route } from "react-router-dom";
import Layout from "./components/Layout";
import Dashboard from "./pages/Dashboard";
import Config from "./pages/Config";
import Logs from "./pages/Logs";

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<Dashboard />} />
        <Route path="/config" element={<Config />} />
        <Route path="/logs" element={<Logs />} />
      </Route>
    </Routes>
  );
}
