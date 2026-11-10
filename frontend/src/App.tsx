import { BrowserRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "./context/AuthContext";
import { Home } from "./pages/Home";
import { FacilityDetail } from "./pages/FacilityDetail";
import { Login } from "./pages/Login";
import { AdminContent } from "./pages/AdminContent";
import { DraftEditor } from "./pages/DraftEditor";
import { Wallet } from "./pages/Wallet";
import { NotFound } from "./pages/NotFound";

function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/" element={<Home />} />
          {/* One reusable template renders all facilities — see
              src/data/facilities.ts for each facility's content. */}
          <Route path="/facilities/:slug" element={<FacilityDetail />} />
          <Route path="/login" element={<Login />} />
          <Route path="/admin/content" element={<AdminContent />} />
          <Route path="/admin/content/new" element={<DraftEditor />} />
          <Route path="/admin/content/:id" element={<DraftEditor />} />
          <Route path="/wallet" element={<Wallet />} />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}

export default App;
