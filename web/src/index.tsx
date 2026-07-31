import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import { CartProvider } from './cart';
import { initTelegram } from './telegram';
import './styles/index.css';

initTelegram();

// ErrorBoundary снаружи роутера: даже падение при инициализации маршрутов
// покажет экран «что-то пошло не так» с выходом в бота, а не белую страницу.
ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <BrowserRouter>
        <CartProvider>
          <App />
        </CartProvider>
      </BrowserRouter>
    </ErrorBoundary>
  </React.StrictMode>,
);
