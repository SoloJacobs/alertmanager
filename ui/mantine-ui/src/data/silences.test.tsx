import { createServer, IncomingMessage, ServerResponse } from 'http';
import { AddressInfo } from 'net';
import React, { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { useSilence, useSilences } from './silences';

class ErrorBoundary extends React.Component<
  { children: ReactNode; onError?: (error: Error) => void },
  { hasError: boolean; error: Error | null }
> {
  constructor(props: { children: ReactNode; onError?: (error: Error) => void }) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error) {
    if (this.props.onError) {
      this.props.onError(error);
    }
  }

  render() {
    if (this.state.hasError) {
      return null;
    }
    return this.props.children;
  }
}

const mockSilence = {
  comment: 'test',
  createdBy: 'Test User',
  endsAt: '2026-03-28T20:00:33.992Z',
  id: '4a1f2ba3-2d27-45ac-bcff-cb5cf04d7b68',
  matchers: [
    { isEqual: true, isRegex: false, name: 'alertname', value: 'alert_annotate' },
    { isEqual: true, isRegex: false, name: 'severity', value: 'critical' },
  ],
  startsAt: '2026-03-28T18:00:38.093Z',
  status: { state: 'active' as const },
  updatedAt: '2026-03-28T18:00:38.093Z',
};

describe('Silence API Hooks', () => {
  let queryClient: QueryClient;
  let respond: (req: IncomingMessage, res: ServerResponse) => void;

  const server = createServer((req, res) => respond(req, res));

  beforeAll(async () => {
    await new Promise<void>((resolve) => server.listen(0, resolve));
    const { port } = server.address() as AddressInfo;
    vi.stubEnv('VITE_API_PREFIX', `http://localhost:${port}`);
  });

  afterAll(async () => {
    vi.unstubAllEnvs();
    await new Promise<void>((resolve) => server.close(resolve));
  });

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    respond = (_, res) => { res.writeHead(500); res.end(); };
  });

  afterEach(() => {
    queryClient.clear();
  });

  const jsonRespond =
    (body: unknown, status = 200) =>
    (_: IncomingMessage, res: ServerResponse) => {
      res.writeHead(status, { 'content-type': 'application/json' });
      res.end(JSON.stringify(body));
    };

  const getWrapper = (client: QueryClient) =>
    ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );

  describe('useSilences - fetch all silences', () => {
    it('should fetch and return array of silences with correct data structure', async () => {
      let requestUrl: string | undefined;
      respond = (req, res) => {
        requestUrl = req.url;
        jsonRespond([mockSilence])(req, res);
      };

      const { result } = renderHook(() => useSilences(), {
        wrapper: getWrapper(queryClient),
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual([mockSilence]);
      expect(Array.isArray(result.current.data)).toBe(true);
      expect(requestUrl).toContain('/api/v2/silences');
    });

    it('should handle empty response', async () => {
      respond = jsonRespond([]);

      const { result } = renderHook(() => useSilences(), {
        wrapper: getWrapper(queryClient),
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(result.current.data).toEqual([]);
    });

    it('should handle API errors (e.g., server returns error status)', async () => {
      respond = jsonRespond({ error: 'Internal server error', status: 'error' });

      const errorCallback = vi.fn();
      const wrapper = ({ children }: { children: ReactNode }) => (
        <ErrorBoundary onError={errorCallback}>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ErrorBoundary>
      );

      renderHook(() => useSilences(), { wrapper });

      await waitFor(() => expect(errorCallback).toHaveBeenCalled());
      expect(errorCallback).toHaveBeenCalledWith(
        expect.objectContaining({ message: expect.stringContaining('Internal server error') })
      );
    });

    it('should handle network errors (e.g., connection refused)', async () => {
      respond = (req) => { req.socket.destroy(); };

      const errorCallback = vi.fn();
      const wrapper = ({ children }: { children: ReactNode }) => (
        <ErrorBoundary onError={errorCallback}>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ErrorBoundary>
      );

      renderHook(() => useSilences(), { wrapper });

      await waitFor(() => expect(errorCallback).toHaveBeenCalled());
    });
  });

  describe('useSilence - fetch single silence by ID', () => {
    const silenceId = '4a1f2ba3-2d27-45ac-bcff-cb5cf04d7b68';

    it('should fetch and return a single silence with correct structure', async () => {
      let requestUrl: string | undefined;
      respond = (req, res) => {
        requestUrl = req.url;
        jsonRespond(mockSilence)(req, res);
      };

      const { result } = renderHook(() => useSilence(silenceId), {
        wrapper: getWrapper(queryClient),
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      expect(result.current.data).toEqual(mockSilence);
      expect(result.current.data).toHaveProperty('id');
      expect(result.current.data).toHaveProperty('status');
      expect(result.current.data).toHaveProperty('matchers');
      expect(requestUrl).toContain(`/api/v2/silence/${silenceId}`);
    });

    it('should handle different silence IDs correctly', async () => {
      const customId = 'custom-silence-id-123';
      let requestUrl: string | undefined;
      respond = (req, res) => {
        requestUrl = req.url;
        jsonRespond({ ...mockSilence, id: customId })(req, res);
      };

      renderHook(() => useSilence(customId), { wrapper: getWrapper(queryClient) });

      await waitFor(() => expect(requestUrl).toBeDefined());
      expect(requestUrl).toContain(`/api/v2/silence/${customId}`);
    });

    it('should handle errors when fetching single silence', async () => {
      respond = jsonRespond({ error: 'Silence not found', status: 'error' });

      const errorCallback = vi.fn();
      const wrapper = ({ children }: { children: ReactNode }) => (
        <ErrorBoundary onError={errorCallback}>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ErrorBoundary>
      );

      renderHook(() => useSilence(silenceId), { wrapper });

      await waitFor(() => expect(errorCallback).toHaveBeenCalled());
      expect(errorCallback).toHaveBeenCalledWith(
        expect.objectContaining({ message: expect.stringContaining('Silence not found') })
      );
    });

    it('should create separate cache entries for different IDs', async () => {
      const id1 = 'id-1';
      const id2 = 'id-2';
      const requestUrls: string[] = [];

      respond = (req, res) => {
        requestUrls.push(req.url!);
        const id = requestUrls.length === 1 ? id1 : id2;
        jsonRespond({ ...mockSilence, id })(req, res);
      };

      const wrapper = getWrapper(queryClient);
      renderHook(() => useSilence(id1), { wrapper });
      renderHook(() => useSilence(id2), { wrapper });

      await waitFor(() => expect(requestUrls.length).toBeGreaterThanOrEqual(2));

      expect(requestUrls.some((u) => u.includes(`/api/v2/silence/${id1}`))).toBe(true);
      expect(requestUrls.some((u) => u.includes(`/api/v2/silence/${id2}`))).toBe(true);
      await waitFor(() => expect(requestUrls).toHaveLength(2));
    });
  });
});
