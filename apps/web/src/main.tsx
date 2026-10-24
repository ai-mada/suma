import React from 'react';
import {createRoot} from 'react-dom/client';
import {App} from './App';
import {ErrorBoundary} from './ErrorBoundary';
import '@xyflow/react/dist/style.css';
import './style.css';
createRoot(document.getElementById('root')!).render(<React.StrictMode><ErrorBoundary><App/></ErrorBoundary></React.StrictMode>);
