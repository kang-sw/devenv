# ws-dashboard frontend

React, TypeScript, and Vite package for the ws web dashboard browser shell.

## Commands

```bash
npm install
npm run dev
npm run build
npm run preview
```

The daemon serves production assets from `frontend/dist` through:

```bash
cargo run -p ws-dashboard-daemon -- serve --static-dir frontend/dist
```
