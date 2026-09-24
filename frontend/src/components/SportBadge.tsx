import { Dynamic } from 'solid-js/web';
import { sportIcon, sportLabel } from '../lib/sports';

export interface SportBadgeProps {
  sport: string;
}

/** SportBadge marks an activity's sport with its icon and name. */
export default function SportBadge(props: SportBadgeProps) {
  return (
    <span class="tag hyl-sport-tag" title={sportLabel(props.sport)}>
      <span class="icon is-small">
        <Dynamic component={sportIcon(props.sport)} size={16} />
      </span>
      <span>{sportLabel(props.sport)}</span>
    </span>
  );
}
