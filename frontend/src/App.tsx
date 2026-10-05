import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Home } from "./pages/Home";
import { FacilityDetail } from "./pages/FacilityDetail";
import { NotFound } from "./pages/NotFound";

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Home />} />
        {/* One reusable template renders all 8 facilities — see
            src/data/facilities.ts for each facility's content. */}
        <Route path="/facilities/:slug" element={<FacilityDetail />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
