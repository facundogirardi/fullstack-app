import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // Redirige /api al backend en Go
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
