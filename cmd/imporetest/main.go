// Package main: 批量诊断账号导入流程，分类失败原因。
//
//	go run ./cmd/imporetest
package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"windsurf-tools-wails/backend/services"
)

type account struct {
	email, password string
}

func main() {
	accs := []account{
		{"nguyenthihongqb1972+zfxgygcl@gmail.com", "eh97xlism3"},
		{"dinhleduc788+bgtqttep@gmail.com", "fawmi1927"},
		{"RuiizzKeinnys839+xuhwyuuw@gmail.com", "28m4bwbr"},
		{"donnaabernathy75+muotunah@gmail.com", "h00cx58eb"},
		{"YeisonSnell757+ddiqfywg@gmail.com", "w2q448l0jsp"},
		{"StathisJockson217+imsqurkt@gmail.com", "vcr8j852t"},
		{"SantiagitoCadrera+wwoujsgp@gmail.com", "4g77b2yya6"},
		{"NicoleGaleener768+daxxnpfm@gmail.com", "r115szf3fv1"},
		{"giannikosstaikos193+gpbyekmz@gmail.com", "30onal8s"},
		{"KrnPina508+ywrkagwl@gmail.com", "rg1wx9o1"},
		{"YeisonSnell757+suscsbos@gmail.com", "igbi03c5sj5"},
		{"lennonhancoc158+imjjhexu@gmail.com", "1kjud9nr3"},
		{"LexHughes984+pufqpbyz@gmail.com", "we0q34tr0"},
		{"DannyMart371+zdjxhfyi@gmail.com", "25x34chhyb2"},
		{"ForcierMackenzie453+fkuhhkot@gmail.com", "u6nnpb492zs"},
		{"ForcierMackenzie453+yjntaquw@gmail.com", "qr40ifp3z"},
		{"JuarezNicole431+smfdbopl@gmail.com", "s58l4so7n"},
		{"vshellabarger+qoykgqww@hawkmail.hccfl.edu", "mzsp138z"},
		{"jairibelio299+mjtgvuaf@gmail.com", "qn5pgw555q"},
		{"KendallBeasley304+wuldfuyp@gmail.com", "cxc09q1j"},
		{"SmithSandivel929+kdrquzuz@gmail.com", "9m379ss9rm"},
		{"GladysPatel808+inyyhsvs@gmail.com", "yq0dbjz60"},
		{"truonghoangdieu7672+bymdxwdu@gmail.com", "d0u6ag4cs"},
		{"KendallBeasley304+qkcwmspv@gmail.com", "qcknre995b"},
		{"NicoleGaleener768+xyzjlcgc@gmail.com", "u41mg336iw"},
		{"YazminMisiak+maabewyk@gmail.com", "x9749jzkjg"},
		{"vohoangtam20943+ocpkppim@gmail.com", "hhn34c4r"},
		{"bvi819315+funflgqg@gmail.com", "6kyrajg746"},
		{"DannyMart371+slmtoncb@gmail.com", "cn51ne11kj"},
		{"GlasgowAndre594+xovfwuco@gmail.com", "05lzr59sv"},
		{"dinhleduc788+qrmwpoer@gmail.com", "p91ogc76dt1"},
		{"nguyendinhthaonguyen18+myslletg@gmail.com", "3p40fzkj"},
		{"vshellabarger+noaoirui@hawkmail.hccfl.edu", "23ww6nkptj"},
		{"donnaabernathy75+xqfephud@gmail.com", "n7zets487qd"},
		{"truonghoangdieu7672+ppreouwg@gmail.com", "g14xp53afcf0"},
		{"KendallBeasley304+gwfizxsy@gmail.com", "4w2lcwl95o"},
		{"MowersKhalid+mpqdrubq@gmail.com", "tq5lt60i"},
		{"GracielaSahai562+sygawngi@gmail.com", "6i6r5owh"},
		{"MaeaLovejoy71+nfngzkku@gmail.com", "8b0knu2f9"},
		{"LexHughes984+caxvxsek@gmail.com", "vpctk777"},
		{"PerssonVicky138+wyfoqnln@gmail.com", "3tx69hl3yn9"},
		{"AvilaNina946+ffeoleaz@gmail.com", "31kge6iad5"},
		{"ngocconglieu7576+qddgkhep@gmail.com", "8v1hgb2g1a"},
		{"dinhleduc788+qsssosjh@gmail.com", "zg69k6ff5"},
		{"JhanselSumon880+zxbeujdr@gmail.com", "faxwtd71780l"},
		{"GlasgowAndre594+nhhlzsce@gmail.com", "8bdk82xu3"},
		{"nguyenvanhung02051996bn+rkmzfurb@gmail.com", "2734il2xmzm"},
		{"nguyendinhthaonguyen18+kmxuulki@gmail.com", "37whfq57z"},
		{"eliannpacheco869+vyribncw@gmail.com", "23xb4h4iw"},
		{"NapolitanoJhon840+ttlpyjud@gmail.com", "0lx7f6cz"},
		{"YeisonSnell757+bqkvxmwj@gmail.com", "c21brgoe991"},
		{"botanadeybis+riywgamq@gmail.com", "c4c0y7akzt89"},
		{"MaeaLovejoy71+darwxxbg@gmail.com", "6lxu9ug4n6y"},
		{"BowmanDjnando433+mydidaoj@gmail.com", "ub6m80eytb23"},
		{"YeisonSnell757+zhbkcouw@gmail.com", "qy6a333jud"},
		{"NapolitanoJhon840+gdzaqndg@gmail.com", "94i2llxv"},
		{"BagepalliSusan706+pwhtjepr@gmail.com", "jw8z0fuq5"},
		{"truongyen99519098+jhdltspm@gmail.com", "impe30486rn"},
		{"MowersKhalid+zldxprsa@gmail.com", "6j6ezj0f97dt"},
		{"amberdelgado7384+veoaiabl@gmail.com", "tdr965cph"},
		{"PerssonVicky138+mfdjkfea@gmail.com", "z1nmg722x7wv"},
		{"QaliJanie807+bucaiezq@gmail.com", "2zh1keo0gf"},
		{"sigouneyenglish825+ufrtxfhf@gmail.com", "10zpij83ogj"},
		{"RuiizzKeinnys839+jrzguono@gmail.com", "8gep7co2nn"},
		{"BagepalliSusan706+zcviupld@gmail.com", "j4mpuw41"},
		{"StreetyConor758+gihxdhtq@gmail.com", "xq4z4kim27f"},
		{"nguyenthihongqb1972+ijhsvuuv@gmail.com", "ux7kn5fm0b"},
		{"QaliJanie807+xwadwxvi@gmail.com", "j3qy71qv"},
		{"coogennevarezod876+mveavsjy@gmail.com", "3f94jdo9b"},
		{"NapolitanoJhon840+usuvnkwq@gmail.com", "0k98e28kzu"},
		{"RosasTina465+pvktanib@gmail.com", "d6b8gyv1uz"},
		{"truongyen99519098+cchwrang@gmail.com", "w4kr756ven"},
		{"YeisonSnell757+banjpbca@gmail.com", "t6r6t7lukl"},
		{"MowersKhalid+lncsgviy@gmail.com", "4v4fh406puu"},
		{"CynthiaStussy878+oodnbkfa@gmail.com", "4doxg570xw"},
		{"RosasTina465+ibpaokyx@gmail.com", "1r6jt70sc"},
		{"RomyJohnson195+qmrxxjoh@gmail.com", "vqf75urh051"},
		{"sampalloeujenia+irsmvsje@gmail.com", "5jj0mwhli3"},
		{"YeisonSnell757+isqoculv@gmail.com", "4zis5hgzp4"},
		{"amberdelgado7384+zoijlbxk@gmail.com", "4q2oxvuw6x78"},
		{"BambiShakiya748+tutjyltp@gmail.com", "zfku47x6"},
		{"AvilaNina946+ktayzrim@gmail.com", "p1z8peea207a"},
		{"SaracayAdamarys561+wmeuntkm@gmail.com", "cwk9clnr56"},
		{"KendallBeasley304+fxgzsxyt@gmail.com", "dwsm852ha"},
		{"BowmanDjnando433+nugzfxsr@gmail.com", "0e3ix8lt65"},
		{"NicoleGaleener768+raswsifm@gmail.com", "n4629kl5gka"},
		{"RuiizzKeinnys839+ybiejeky@gmail.com", "g92f7vj2v6na"},
		{"donnaabernathy75+eimpinan@gmail.com", "9pm5f0rrxx"},
		{"eliannpacheco869+wjnlqvtl@gmail.com", "q8c01yii0z4"},
		{"CraikConnie760+krtetdxs@gmail.com", "9j33bwxd7"},
		{"mowedaijxhoaevmsop8260+ygcybpwy@gmail.com", "y0f6cs92o"},
		{"LyssBanks663+yruicjyk@gmail.com", "wuz60p0w0"},
		{"LexHughes984+gjvwlddh@gmail.com", "7n047ik3rnc"},
		{"StubsTucker91+lbktouil@gmail.com", "g2ib8o7y4h"},
		{"AguilarPorwal53+lqwmghlc@gmail.com", "86kdjdqs612"},
		{"SmithSandivel929+ebjfoanp@gmail.com", "a59t5hksw"},
		{"santoshkumar6207145+cdvcdpyo@gmail.com", "w8uj0adu05"},
		{"wajihamaung+ahvsbdfa@gmail.com", "6t0w4tqi7"},
		{"QaliJanie807+zzkkdcci@gmail.com", "ue3s0f3m3a"},
		{"NicoleGaleener768+awolcpoj@gmail.com", "gvt92f95hz"},
	}

	svc := services.NewWindsurfService("")

	type result struct {
		idx     int
		email   string
		stage   string // login | postauth | jwt | quota | OK
		ok      bool
		summary string
	}
	results := make([]result, len(accs))
	var done int32

	var wg sync.WaitGroup
	sem := make(chan struct{}, 1)
	for i, a := range accs {
		wg.Add(1)
		go func(idx int, ac account) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// 每账号总超时 60s，避免单号卡死拖整批
			ch := make(chan struct {
				idx     int
				email   string
				stage   string
				ok      bool
				summary string
			}, 1)
			go func() { ch <- process(svc, idx, ac) }()
			var r struct {
				idx     int
				email   string
				stage   string
				ok      bool
				summary string
			}
			select {
			case r = <-ch:
			case <-time.After(120 * time.Second):
				r.idx = idx
				r.email = ac.email
				r.stage = "timeout"
				r.summary = "总耗时超过 120s"
			}
			results[idx] = r
			n := atomic.AddInt32(&done, 1)
			fmt.Printf("[%2d/%d] %-50s %s | %s\n", n, len(accs), trimEmail(ac.email),
				okMark(r.ok, r.stage), r.summary)
		}(i, a)
	}
	wg.Wait()

	// 分类汇总
	stages := map[string]int{}
	stageErrs := map[string][]string{} // stage -> list of "[idx] email — err"
	ok := 0
	for _, r := range results {
		stages[r.stage]++
		if !r.ok {
			stageErrs[r.stage] = append(stageErrs[r.stage], fmt.Sprintf("  [%2d] %s — %s", r.idx+1, r.email, r.summary))
		} else {
			ok++
		}
	}

	fmt.Println("\n========== 分类汇总 ==========")
	fmt.Printf("成功: %d / %d\n", ok, len(accs))
	for _, s := range []string{"login", "postauth", "jwt", "quota"} {
		if len(stageErrs[s]) > 0 {
			fmt.Printf("\n失败阶段 [%s]: %d 条\n", s, len(stageErrs[s]))
			for _, e := range stageErrs[s] {
				fmt.Println(e)
			}
		}
	}
}

func okMark(ok bool, stage string) string {
	if ok {
		return "✅ OK"
	}
	return "❌ " + stage
}

func trimEmail(e string) string {
	if len(e) > 50 {
		return e[:47] + "..."
	}
	return e
}

func process(svc *services.WindsurfService, idx int, a account) (r struct {
	idx     int
	email   string
	stage   string
	ok      bool
	summary string
}) {
	r.idx = idx
	r.email = a.email
	t0 := time.Now()

	resp, err := svc.LoginWithEmail(a.email, a.password)
	if err != nil {
		r.stage = "login"
		r.summary = condense(err.Error())
		return
	}
	tok := strings.TrimSpace(resp.IDToken)
	loginKind := "firebase"
	if strings.HasPrefix(tok, "auth1_") {
		loginKind = "auth1"
	}

	var apiKey string
	if loginKind == "auth1" {
		pa, err := svc.WindsurfPostAuth(tok)
		if err != nil {
			r.stage = "postauth"
			r.summary = condense(err.Error())
			return
		}
		apiKey = pa.SessionKey
	} else {
		reg, err := svc.RegisterUser(tok)
		if err != nil {
			r.stage = "register"
			r.summary = condense(err.Error())
			return
		}
		apiKey = reg.APIKey
	}
	if apiKey == "" {
		r.stage = "postauth"
		r.summary = "未拿到 apiKey"
		return
	}

	jwt, err := svc.GetJWTByAPIKey(apiKey)
	if err != nil {
		r.stage = "jwt"
		r.summary = fmt.Sprintf("%s GetJWTByAPIKey: %s", loginKind, condense(err.Error()))
		return
	}
	_ = jwt

	prof, err := svc.GetUserStatus(apiKey)
	if err != nil {
		r.stage = "quota"
		r.summary = fmt.Sprintf("%s GetUserStatus: %s", loginKind, condense(err.Error()))
		return
	}

	r.stage = "OK"
	r.ok = true
	r.summary = fmt.Sprintf("%s plan=%s used=%d/%d (耗时 %s)",
		loginKind, prof.PlanName, prof.UsedCredits, prof.TotalCredits, time.Since(t0).Round(time.Millisecond))
	return
}

func condense(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 600 {
		s = s[:600] + "..."
	}
	return s
}
