/** NotFoundPage is the catch-all SPA route. */
export default function NotFoundPage() {
  return (
    <section class="section">
      <div class="container has-text-centered">
        <h1 class="title">Page not found</h1>
        <p class="subtitle is-6">That link does not point at anything here.</p>
        <a class="button is-primary" href="/">
          Back to the feed
        </a>
      </div>
    </section>
  );
}
