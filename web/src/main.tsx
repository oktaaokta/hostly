import React from 'react';
import ReactDOM from 'react-dom/client';
import CustomerPage from './CustomerPage';
import StaffPage from './StaffPage';
import './styles.css';

function NotFound() {
  return (
    <main className="center-page">
      <h1>Not found</h1>
      <p>This link doesn't point anywhere.</p>
    </main>
  );
}

function App() {
  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  const path = base ? window.location.pathname.replace(new RegExp('^' + base), '') : window.location.pathname;
  const m = path.match(/^\/(q|staff)\/([^/]+)/);
  if (!m) return <NotFound />;
  const slug = decodeURIComponent(m[2]);
  if (m[1] === 'q') return <CustomerPage slug={slug} />;
  const token = new URLSearchParams(window.location.search).get('token') ?? '';
  return <StaffPage slug={slug} token={token} />;
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);