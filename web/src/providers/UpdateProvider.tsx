'use client';

import { createContext, useContext, useEffect, useRef, ReactNode, useCallback } from 'react';
import { dispatcher, Update } from '@/lib/updateDispatcher';

type SubscriberFn = (update: Update) => void;

interface UpdateContextValue {
  subscribe: (topic: string, fn: SubscriberFn) => () => void;
}

const UpdateContext = createContext<UpdateContextValue>({
  subscribe: () => () => {},
});

export function useUpdateDispatcher() {
  return useContext(UpdateContext);
}

/**
 * Hook to subscribe to a topic. Uses a ref to avoid re-subscription when
 * the handler changes (e.g. when it depends on component state).
 */
export function useSubscribe(topic: string, onMessage: (update: Update) => void) {
  const { subscribe } = useUpdateDispatcher();
  const handlerRef = useRef(onMessage);
  handlerRef.current = onMessage;

  useEffect(() => {
    return subscribe(topic, (update) => handlerRef.current(update));
  }, [topic, subscribe]);
}

export function UpdateProvider({ children }: { children: ReactNode }) {
  const subscribe = useCallback((topic: string, fn: SubscriberFn) => {
    return dispatcher.subscribe(topic, fn);
  }, []);

  return (
    <UpdateContext.Provider value={{ subscribe }}>
      {children}
    </UpdateContext.Provider>
  );
}
