import { BrowserRouter, Routes, Route } from "react-router-dom";
import { AuthProvider } from "./context/AuthContext";
import { Home } from "./pages/Home";
import { FacilityDetail } from "./pages/FacilityDetail";
import { Login } from "./pages/Login";
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
          <Route path="*" element={<NotFound />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}

export default App;
