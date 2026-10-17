import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./i18n";
import "./index.css";

const theme = localStorage.getItem("datasafe-desktop.theme") || "dark";
const systemDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
document.documentElement.classList.toggle("dark", theme !== "light" && (theme === "dark" || systemDark));
document.documentElement.lang = localStorage.getItem("datasafe-desktop.locale") === "ru" ? "ru" : "en";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
