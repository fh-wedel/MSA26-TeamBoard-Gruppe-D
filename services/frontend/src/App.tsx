import { Route, Routes } from "react-router-dom";
import UserView from "./pages/UserView";
import AdminView from "./pages/AdminView";

export default function App(): JSX.Element {
  return (
    <Routes>
      <Route path="/" element={<UserView />} />
      <Route path="/admin" element={<AdminView />} />
      <Route path="*" element={<UserView />} />
    </Routes>
  );
}
