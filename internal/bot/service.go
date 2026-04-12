package bot

import "fmt"

type Service struct {
	store *Store
}

func NewService(store *Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("bot store is required")
	}
	return &Service{store: store}, nil
}

func (s *Service) List(channel string) ([]Bot, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("bot store is required")
	}
	if channel == "" {
		return s.store.List(), nil
	}

	normalized, err := NormalizeChannel(channel)
	if err != nil {
		return nil, err
	}

	all := s.store.List()
	filtered := make([]Bot, 0, len(all))
	for _, b := range all {
		if b.Channel == string(normalized) {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}
