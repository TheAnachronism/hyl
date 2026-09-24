import { For, Show } from 'solid-js';
import { count } from '../lib/format';

export interface PagerProps {
  page: number;
  totalPages: number;
  total: number;
  /** hrefFor builds the URL of a page, so a caller keeps its own filters. */
  hrefFor: (page: number) => string;
}

/** WINDOW is how many pages around the current one are always shown. */
const NEIGHBOURS = 1;
const PLAIN_LIMIT = 7;

/**
 * pageWindow lists the page numbers to render, with 0 standing for an elided
 * run, so a long list stays short: 1 … 6 7 8 … 24
 */
function pageWindow(current: number, total: number): number[] {
  if (total <= PLAIN_LIMIT) return Array.from({ length: total }, (_, index) => index + 1);

  const wanted = new Set<number>([1, total, current]);
  for (let offset = 1; offset <= NEIGHBOURS; offset += 1) {
    wanted.add(current - offset);
    wanted.add(current + offset);
  }
  // Keep the row the same width when the current page sits at either end.
  if (current <= NEIGHBOURS + 2) [2, 3, 4].forEach((page) => wanted.add(page));
  if (current >= total - (NEIGHBOURS + 1)) [total - 3, total - 2, total - 1].forEach((page) => wanted.add(page));

  const pages = [...wanted].filter((page) => page >= 1 && page <= total).sort((a, b) => a - b);
  const out: number[] = [];
  let previous = 0;
  for (const page of pages) {
    if (previous !== 0 && page - previous > 1) out.push(0);
    out.push(page);
    previous = page;
  }
  return out;
}

/** Pager says which page you are on and moves between pages. */
export default function Pager(props: PagerProps) {
  const hasPrevious = () => props.page > 1;
  const hasNext = () => props.page < props.totalPages;

  return (
    <Show when={props.totalPages > 1}>
      <nav class="hyl-pager" role="navigation" aria-label="Pagination">
        <p class="hyl-pager-summary has-text-grey is-size-7">
          Page {props.page} of {props.totalPages} · {count(props.total)}{' '}
          {props.total === 1 ? 'activity' : 'activities'}
        </p>

        <nav class="pagination is-small" role="navigation" aria-label="Pages">
          <Show
            when={hasPrevious()}
            fallback={<span class="pagination-previous hyl-pager-off">Previous</span>}
          >
            <a class="pagination-previous" href={props.hrefFor(props.page - 1)} rel="prev">
              Previous
            </a>
          </Show>
          <Show when={hasNext()} fallback={<span class="pagination-next hyl-pager-off">Next</span>}>
            <a class="pagination-next" href={props.hrefFor(props.page + 1)} rel="next">
              Next
            </a>
          </Show>

          <ul class="pagination-list">
            <For each={pageWindow(props.page, props.totalPages)}>
              {(page) => (
                <li>
                  <Show when={page !== 0} fallback={<span class="pagination-ellipsis">&hellip;</span>}>
                    <Show
                      when={page !== props.page}
                      fallback={
                        <span class="pagination-link is-current" aria-current="page">
                          {page}
                        </span>
                      }
                    >
                      <a class="pagination-link" href={props.hrefFor(page)} aria-label={`Page ${page}`}>
                        {page}
                      </a>
                    </Show>
                  </Show>
                </li>
              )}
            </For>
          </ul>
        </nav>
      </nav>
    </Show>
  );
}
