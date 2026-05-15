export function App() {
  return (
    <main className="app-shell" aria-label="ws dashboard">
      <div className="shell-grid">
        <aside className="shell-panel shell-panel-nav">
          <div className="panel-title">ws dashboard</div>
        </aside>
        <section className="shell-panel shell-panel-main">
          <div className="panel-title">Dashboard</div>
        </section>
        <aside className="shell-panel shell-panel-viewer">
          <div className="panel-title">Viewer</div>
        </aside>
      </div>
    </main>
  );
}
