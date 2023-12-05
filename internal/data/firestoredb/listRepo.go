package firestoredb

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
)

type listRepo struct {
	ctx        *context.Context
	white_coll string
	black_coll string
	client     *firestore.Client
}

func (w listRepo) StoreWhiteList(wl *nostr.WhiteList) error {

	if _, err := w.client.Collection(w.white_coll).Doc(wl.PubKey).Create(*w.ctx, wl); err != nil {
		return fmt.Errorf("unable to save whitelist repository. error: %v", err)
	}
	return nil
}

func (w listRepo) GetWhiteList(pbk string) (*nostr.WhiteList, error) {

	var wlr nostr.WhiteList
	doc, err := w.client.Collection(w.white_coll).Doc(pbk).Get(*w.ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get whitelist from repository. error: %v", err)
	}

	if err := doc.DataTo(&wlr); err != nil {
		return nil, fmt.Errorf("unable to fit whitelist format. error: %v", err)
	}

	return &wlr, nil
}

func (w listRepo) GetWhiteLists(pbks []string) ([]*nostr.WhiteList, error) {
	// [ ]: (on demand) implement on demand
	return nil, nil
}

func NewListRepository(ctx *context.Context, client *firestore.Client, wlcn, blcn string) (nostr.ListRepo, error) {
	return &listRepo{
		ctx:        ctx,
		white_coll: wlcn,
		black_coll: blcn,
		client:     client,
	}, nil
}

// ================================= BLACK LIST =================================

func (b listRepo) StoreBlackList(bl *nostr.BlackList) error {

	if _, err := b.client.Collection(b.black_coll).Doc(bl.IP).Set(*b.ctx, bl); err != nil {
		return fmt.Errorf("unable to save blacklist repository. error: %v", err)
	}
	return nil
}

func (b listRepo) GetBlackList(ip string) (*nostr.BlackList, error) {

	var blr nostr.BlackList
	doc, err := b.client.Collection(b.black_coll).Doc(ip).Get(*b.ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get blacklist from repository. error: %v", err)
	}

	if err := doc.DataTo(&blr); err != nil {
		return nil, fmt.Errorf("unable to fit blacklist format. error: %v", err)
	}

	return &blr, nil
}

func (b listRepo) GetBlackLists(ips []string) ([]*nostr.BlackList, error) {

	// [ ]: (on demand) implement on demand
	return nil, nil
}
