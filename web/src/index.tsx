import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';
import { CartProvider } from './cart';
import { initTelegram, onThemeChanged } from './telegram';
import { applyTheme, currentTheme } from './theme';
import './styles/index.css';

initTelegram();
// Тему применяем до первой отрисовки, иначе на старте мелькает светлый фон.
applyTheme(currentTheme());
onThemeChanged(() => applyTheme(currentTheme()));

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <CartProvider>
        <App />
      </CartProvider>
    </BrowserRouter>
  </React.StrictMode>,
);
