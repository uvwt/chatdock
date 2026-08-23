import { useCallback, useEffect, useRef, useState } from 'react';
import { fetchPinned, fetchSessions, searchSessions } from '../lib/sessionApi.js';
import { mergeSessionPages, normalizeSessionPage, removeSessionSummary, SESSION_PAGE_SIZE, sessionMatchesProjectFilter, upsertSessionSummary } from '../lib/sessionPagination.js';

function normalizePinnedFeed(data = {}) {
  return {
    sessions: Array.isArray(data.sessions) ? data.sessions : [],
    projects: Array.isArray(data.projects) ? data.projects : [],
    tasks: Array.isArray(data.tasks) ? data.tasks : [],
  };
}

function upsertPinnedItem(items, item) {
  if (!item?.id) return Array.isArray(items) ? items : [];
  const rest = (Array.isArray(items) ? items : []).filter(current => current.id !== item.id);
  if (!item.pinned) return rest;
  return [item, ...rest];
}

export function useSessionList(api, projectFilter = 'all') {
  const [sessions, setSessions] = useState([]);
  // 侧栏必须区分“还没加载完”和“确实没有会话”。缺少这个标记时，首屏和切换筛选都会
  // 先渲染一帧“暂无会话”再被真实列表替换，看起来就是列表闪一下。
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  const [pinnedSessions, setPinnedSessions] = useState([]);
  const [pinnedProjects, setPinnedProjects] = useState([]);
  const [pinnedTasks, setPinnedTasks] = useState([]);
  // 置顶区同样要区分“还没取到 pinned feed”和“确实没有置顶项”：只看数组长度会让整段
  // 从不渲染跳到渲染，把下面的项目、定时任务和全部会话整体往下推一次。
  const [pinnedLoaded, setPinnedLoaded] = useState(false);
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
  const pinnedRequestRef = useRef(0);
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
    // 这里不清空 sessions：切换项目筛选时保留上一批结果，等新一页返回后整体替换，
    // 避免列表先塌成空态再重新长出来。加载状态交给 sessionsLoaded 表达。
    setSessionsLoaded(false);
    setSessionsHasMore(false);
    setSessionsLoadingMore(false);
    setSessionSearchResults([]);
    setSessionSearchBusy(false);
    setSessionSearchHasMore(false);
    setSessionSearchLoadingMore(false);
  }, [api, projectFilter]);

  useEffect(() => {
    pinnedRequestRef.current += 1;
    // 同样不清空置顶数据，只把它标记为未加载：清空会让整段置顶区先消失再出现。
    setPinnedLoaded(false);
  }, [api]);

  const loadPinnedFeed = useCallback(async () => {
    const requestID = pinnedRequestRef.current + 1;
    pinnedRequestRef.current = requestID;
    try {
      const feed = normalizePinnedFeed(await fetchPinned(api));
      if (requestID !== pinnedRequestRef.current) return feed;
      setPinnedSessions(feed.sessions);
      setPinnedProjects(feed.projects);
      setPinnedTasks(feed.tasks);
      return feed;
    } catch {
      if (requestID === pinnedRequestRef.current) {
        setPinnedSessions([]);
        setPinnedProjects([]);
        setPinnedTasks([]);
      }
      return normalizePinnedFeed();
    } finally {
      // 成功和失败都要退出加载态，否则请求出错时侧栏会永久停在骨架上。
      if (requestID === pinnedRequestRef.current) setPinnedLoaded(true);
    }
  }, [api]);

  const loadSessions = useCallback(async ({ reset = false } = {}) => {
    const requestID = listRequestRef.current + 1;
    listRequestRef.current = requestID;
    listRefreshingRef.current = true;
    const targetPages = reset ? 1 : Math.max(1, loadedPagesRef.current);
    const pinnedPromise = loadPinnedFeed();
    try {
      let cursor = '';
      let items = [];
      let hasMore = true;
      let loadedPages = 0;
      for (let pageIndex = 0; pageIndex < targetPages && hasMore; pageIndex += 1) {
        const page = normalizeSessionPage(await fetchSessions(api, {
          cursor,
          limit: SESSION_PAGE_SIZE,
          projectFilter,
          pinned: false,
        }));
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
      await pinnedPromise;
      return items;
    } finally {
      // 只有仍然属于当前筛选的请求才结束加载态；请求失败时也要置位，
      // 否则侧栏会永久停在骨架上，比展示上一批结果更糟。
      if (requestID === listRequestRef.current) {
        listRefreshingRef.current = false;
        setSessionsLoaded(true);
      }
    }
  }, [api, loadPinnedFeed, projectFilter]);

  const loadMoreSessions = useCallback(async () => {
    if (!listHasMoreRef.current || listLoadingMoreRef.current || listRefreshingRef.current) return;
    const requestID = listRequestRef.current;
    listLoadingMoreRef.current = true;
    setSessionsLoadingMore(true);
    try {
      const activeFilter = projectFilterRef.current;
      const page = normalizeSessionPage(await fetchSessions(api, {
        cursor: listCursorRef.current,
        limit: SESSION_PAGE_SIZE,
        projectFilter: activeFilter,
        pinned: false,
      }));
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
    setPinnedSessions(current => upsertSessionSummary(current, session, { requirePinned: true }));
    setSessions(current => sessionMatchesProjectFilter(session, projectFilterRef.current)
      ? upsertSessionSummary(current, session, { requirePinned: false })
      : removeSessionSummary(current, session?.id));
  }, []);

  const upsertPinnedProject = useCallback(project => {
    setPinnedProjects(current => upsertPinnedItem(current, project));
  }, []);

  const upsertPinnedTask = useCallback(task => {
    setPinnedTasks(current => upsertPinnedItem(current, task));
  }, []);

  const removeSession = useCallback(sessionID => {
    setPinnedSessions(current => removeSessionSummary(current, sessionID));
    setSessions(current => removeSessionSummary(current, sessionID));
    setSessionSearchResults(current => removeSessionSummary(current, sessionID));
  }, []);

  return {
    sessions,
    sessionsLoaded,
    pinnedSessions,
    pinnedProjects,
    pinnedTasks,
    pinnedLoaded,
    sessionSearch,
    setSessionSearch,
    sessionSearchResults,
    sessionSearchBusy,
    sessionsHasMore,
    sessionsLoadingMore,
    sessionSearchHasMore,
    sessionSearchLoadingMore,
    loadSessions,
    loadPinnedFeed,
    loadMoreSessions,
    loadMoreSearchSessions,
    upsertSession,
    upsertPinnedProject,
    upsertPinnedTask,
    removeSession,
  };
}
