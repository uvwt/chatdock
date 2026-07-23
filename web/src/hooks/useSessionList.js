import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchSessions, searchSessions } from '../lib/sessionApi.js';
import { mergeSessionPages, normalizeSessionPage, SESSION_PAGE_SIZE, upsertSessionSummary } from '../lib/sessionPagination.js';

export function useSessionList(api, projectFilter = 'all') {
  const [sessions, setSessions] = useState([]);
  const [sessionsHasMore, setSessionsHasMore] = useState(false);
  const [sessionsLoadingMore, setSessionsLoadingMore] = useState(false);
  const [sessionSearch, setSessionSearch] = useState('');
  const [sessionSearchResults, setSessionSearchResults] = useState([]);
  const [sessionSearchBusy, setSessionSearchBusy] = useState(false);
  const [sessionSearchHasMore, setSessionSearchHasMore] = useState(false);
  const [sessionSearchLoadingMore, setSessionSearchLoadingMore] = useState(false);

  const listCursorRef = useRef('');
  const listHasMoreRef = useRef(false);
  const listLoadingMoreRef = useRef(false);
  const listRefreshingRef = useRef(false);
  const loadedPagesRef = useRef(1);
  const listRequestRef = useRef(0);
  const searchCursorRef = useRef('');
  const searchHasMoreRef = useRef(false);
  const searchLoadingMoreRef = useRef(false);
  const searchRequestRef = useRef(0);
  const searchValueRef = useRef('');
  const projectFilterRef = useRef(projectFilter);

  useEffect(() => {
    projectFilterRef.current = projectFilter;
    listRequestRef.current += 1;
    searchRequestRef.current += 1;
    listCursorRef.current = '';
    listHasMoreRef.current = false;
    listLoadingMoreRef.current = false;
    listRefreshingRef.current = false;
    loadedPagesRef.current = 1;
    searchCursorRef.current = '';
    searchHasMoreRef.current = false;
    searchLoadingMoreRef.current = false;
    setSessions([]);
    setSessionsHasMore(false);
    setSessionsLoadingMore(false);
    setSessionSearchResults([]);
    setSessionSearchBusy(false);
    setSessionSearchHasMore(false);
    setSessionSearchLoadingMore(false);
  }, [api, projectFilter]);

  const loadSessions = useCallback(async ({ reset = false } = {}) => {
    const requestID = listRequestRef.current + 1;
    listRequestRef.current = requestID;
    listRefreshingRef.current = true;
    const targetPages = reset ? 1 : Math.max(1, loadedPagesRef.current);
    try {
      let cursor = '';
      let items = [];
      let hasMore = true;
      let loadedPages = 0;
      for (let pageIndex = 0; pageIndex < targetPages && hasMore; pageIndex += 1) {
        const page = normalizeSessionPage(await fetchSessions(api, { cursor, limit: SESSION_PAGE_SIZE, projectFilter }));
        if (requestID !== listRequestRef.current || projectFilter !== projectFilterRef.current) return [];
        items = mergeSessionPages(items, page.sessions);
        cursor = page.nextCursor;
        hasMore = page.hasMore;
        loadedPages += 1;
      }
      if (requestID !== listRequestRef.current || projectFilter !== projectFilterRef.current) return [];
      listCursorRef.current = cursor;
      listHasMoreRef.current = hasMore;
      loadedPagesRef.current = Math.max(1, loadedPages);
      setSessions(items);
      setSessionsHasMore(hasMore);
      return items;
    } finally {
      if (requestID === listRequestRef.current) listRefreshingRef.current = false;
    }
  }, [api, projectFilter]);

  const loadMoreSessions = useCallback(async () => {
    if (!listHasMoreRef.current || listLoadingMoreRef.current || listRefreshingRef.current) return;
    const requestID = listRequestRef.current;
    listLoadingMoreRef.current = true;
    setSessionsLoadingMore(true);
    try {
      const activeFilter = projectFilterRef.current;
      const page = normalizeSessionPage(await fetchSessions(api, { cursor: listCursorRef.current, limit: SESSION_PAGE_SIZE, projectFilter: activeFilter }));
      if (requestID !== listRequestRef.current || activeFilter !== projectFilterRef.current) return;
      setSessions(current => mergeSessionPages(current, page.sessions));
      listCursorRef.current = page.nextCursor;
      listHasMoreRef.current = page.hasMore;
      loadedPagesRef.current += 1;
      setSessionsHasMore(page.hasMore);
    } catch {
      // 保留当前页和游标，下一次滚动可以继续重试。
    } finally {
      listLoadingMoreRef.current = false;
      setSessionsLoadingMore(false);
    }
  }, [api]);

  useEffect(() => {
    const query = sessionSearch.trim();
    searchValueRef.current = query;
    const requestID = searchRequestRef.current + 1;
    searchRequestRef.current = requestID;
    searchCursorRef.current = '';
    searchHasMoreRef.current = false;
    searchLoadingMoreRef.current = false;
    setSessionSearchResults([]);
    setSessionSearchHasMore(false);
    setSessionSearchLoadingMore(false);
    if (!query) {
      setSessionSearchBusy(false);
      return undefined;
    }

    setSessionSearchBusy(true);
    const timer = window.setTimeout(async () => {
      try {
        const activeFilter = projectFilterRef.current;
        const page = normalizeSessionPage(await searchSessions(api, query, { limit: SESSION_PAGE_SIZE, projectFilter: activeFilter }));
        if (requestID !== searchRequestRef.current || query !== searchValueRef.current || activeFilter !== projectFilterRef.current) return;
        setSessionSearchResults(page.sessions);
        searchCursorRef.current = page.nextCursor;
        searchHasMoreRef.current = page.hasMore;
        setSessionSearchHasMore(page.hasMore);
      } catch {
        if (requestID === searchRequestRef.current) setSessionSearchResults([]);
      } finally {
        if (requestID === searchRequestRef.current) setSessionSearchBusy(false);
      }
    }, 260);
    return () => window.clearTimeout(timer);
  }, [api, sessionSearch, projectFilter]);

  const loadMoreSearchSessions = useCallback(async () => {
    const query = searchValueRef.current;
    const requestID = searchRequestRef.current;
    if (!query || !searchHasMoreRef.current || searchLoadingMoreRef.current) return;
    searchLoadingMoreRef.current = true;
    setSessionSearchLoadingMore(true);
    try {
      const activeFilter = projectFilterRef.current;
      const page = normalizeSessionPage(await searchSessions(api, query, { cursor: searchCursorRef.current, limit: SESSION_PAGE_SIZE, projectFilter: activeFilter }));
      if (requestID !== searchRequestRef.current || query !== searchValueRef.current || activeFilter !== projectFilterRef.current) return;
      setSessionSearchResults(current => mergeSessionPages(current, page.sessions));
      searchCursorRef.current = page.nextCursor;
      searchHasMoreRef.current = page.hasMore;
      setSessionSearchHasMore(page.hasMore);
    } catch {
      // 保留当前搜索结果，下一次滚动继续重试。
    } finally {
      searchLoadingMoreRef.current = false;
      setSessionSearchLoadingMore(false);
    }
  }, [api]);

  const upsertSession = useCallback(session => {
    setSessions(current => upsertSessionSummary(current, session));
  }, []);

  const removeSession = useCallback(sessionID => {
    setSessions(current => current.filter(session => session.id !== sessionID));
    setSessionSearchResults(current => current.filter(session => session.id !== sessionID));
  }, []);

  return {
    sessions,
    sessionSearch,
    setSessionSearch,
    sessionSearchResults,
    sessionSearchBusy,
    sessionsHasMore,
    sessionsLoadingMore,
    sessionSearchHasMore,
    sessionSearchLoadingMore,
    loadSessions,
    loadMoreSessions,
    loadMoreSearchSessions,
    upsertSession,
    removeSession,
  };
}
