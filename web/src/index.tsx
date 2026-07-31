import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import { CartProvider } from './cart';
import { initTelegram } from './telegram';
import { captureSource } from './source';
import './styles/index.css';

initTelegram();
// Метку канала снимаем до первой навигации: в start_param она приходит
// только при запуске Mini App, дальше её уже не спросить.
captureSource();

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <CartProvider>
        <App />
      </CartProvider>
    </BrowserRouter>
  </React.StrictMode>,
);
