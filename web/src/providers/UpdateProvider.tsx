'use client';

import { createContext, useContext, useEffect, ReactNode, useCallback } from 'react';
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
 * Hook to subscribe to a topic. Mount → subscribe, unmount → unsubscribe.
 */
export function useSubscribe(topic: string, onMessage: (update: Update) => void) {
  const { subscribe } = useUpdateDispatcher();
  useEffect(() => {
    return subscribe(topic, onMessage);
  }, [topic, onMessage, subscribe]);
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
