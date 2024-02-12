package firestoredb

import (
	"context"
	"fmt"

	"github.com/dasiyes/ivmostr-tdd/internal/nostr"
	"github.com/dasiyes/ivmostr-tdd/pkg/fspool"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
)

type listRepo struct {
	ctx        *context.Context
	lgr        *log.Logger
	white_coll string
	black_coll string
	clients    *fspool.ConnectionPool
}

func (w listRepo) StoreWhiteList(wl *nostr.WhiteList) error {
	fsclient, err := w.clients.GetClient()
	if err != nil {
		return fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	//defer w.clients.ReleaseClient(fsclient)

	if _, err := fsclient.Collection(w.white_coll).Doc(wl.PubKey).Create(*w.ctx, wl); err != nil {
		return fmt.Errorf("unable to save whitelist repository. error: %v", err)
	}
	return nil
}

func (w listRepo) GetWhiteList(pbk string) (*nostr.WhiteList, error) {

	fsclient, err := w.clients.GetClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	defer w.clients.ReleaseClient(fsclient)

	var wlr nostr.WhiteList
	doc, err := fsclient.Collection(w.white_coll).Doc(pbk).Get(*w.ctx)
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

func (w listRepo) GetWLIPS() ([]string, error) {

	var wlips = []string{}

	fsclient, err := w.clients.GetClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	defer w.clients.ReleaseClient(fsclient)

	query := fsclient.Collection(w.white_coll).Where("IP", "!=", "").Documents(*w.ctx)

	var wlr nostr.WhiteList
	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			w.lgr.Errorf("[GetWLIPS] ERROR raised while reading a doc from the DB: %v", err)
			continue
		}

		if err := doc.DataTo(&wlr); err != nil {
			w.lgr.Errorf("[GetWLIPS] ERROR raised while fitting whitelist format: %v", err)
			continue
		}

		wlips = append(wlips, wlr.IP)
	}
	return wlips, nil
}

func NewListRepository(ctx *context.Context, clients *fspool.ConnectionPool, wlcn, blcn string) (nostr.ListRepo, error) {
	return &listRepo{
		ctx:        ctx,
		lgr:        log.StandardLogger(),
		white_coll: wlcn,
		black_coll: blcn,
		clients:    clients,
	}, nil
}

// ================================= BLACK LIST =================================

func (b listRepo) StoreBlackList(bl *nostr.BlackList) error {

	fsclient, err := b.clients.GetClient()
	if err != nil {
		return fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	//defer b.clients.ReleaseClient(fsclient)

	if _, err := fsclient.Collection(b.black_coll).Doc(bl.IP).Set(*b.ctx, bl); err != nil {
		return fmt.Errorf("unable to save blacklist repository. error: %v", err)
	}
	return nil
}

func (b listRepo) GetBlackList(ip string) (*nostr.BlackList, error) {

	fsclient, err := b.clients.GetClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	defer b.clients.ReleaseClient(fsclient)

	var blr nostr.BlackList
	doc, err := fsclient.Collection(b.black_coll).Doc(ip).Get(*b.ctx)
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

func (b listRepo) GetBLIPS() ([]string, error) {

	var blips = []string{}

	fsclient, err := b.clients.GetClient()
	if err != nil {
		return nil, fmt.Errorf("unable to get firestore client. error: %v", err)
	}
	defer b.clients.ReleaseClient(fsclient)

	query := fsclient.Collection(b.black_coll).Where("IP", "!=", "").Documents(*b.ctx)

	var blr nostr.BlackList
	for {
		doc, err := query.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			b.lgr.Errorf("[GetBLIPS] ERROR raised while reading a doc from the DB: %v", err)
			continue
		}

		if err := doc.DataTo(&blr); err != nil {
			b.lgr.Errorf("[GetBLIPS] ERROR raised while fitting blacklist format: %v", err)
			continue
		}

		blips = append(blips, blr.IP)
	}
	return blips, nil
}
